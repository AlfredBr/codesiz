package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type FileData struct {
	Path      string `json:"path"`
	LineCount int    `json:"line_count"`
}

type AnalysisResult struct {
	TotalFiles    int             `json:"total_files"`
	Average       float64         `json:"average"`
	Median        float64         `json:"median"`
	StdDevHigh    float64         `json:"std_dev_high"`
	StdDevLow     float64         `json:"std_dev_low"`
	TotalSum      int             `json:"total_sum"`
	SmallestFile  FileData        `json:"smallest_file"`
	LargestFile   FileData        `json:"largest_file"`
	Files         []FileData      `json:"files,omitempty"`
	Clusters      []ClusterResult `json:"clusters,omitempty"`
	ExcludedFiles []FileData      `json:"excluded_files,omitempty"`
}

type gitignoreRule struct {
	Pattern string
	Negate  bool
}

func analyze(opts Options, config *Config) (AnalysisResult, error) {
	if err := validateRoot(opts.Root); err != nil {
		return AnalysisResult{}, err
	}
	gitignore, err := loadGitignore(opts.Root)
	if err != nil {
		return AnalysisResult{}, err
	}

	var files []FileData
	err = filepath.WalkDir(opts.Root, func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(opts.Root, filePath)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		if relPath == "." {
			return nil
		}
		if d.IsDir() {
			if gitignore.ignores(relPath, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if gitignore.ignores(relPath, false) || !shouldAnalyze(relPath, opts, config) {
			return nil
		}
		lines, err := countLines(filePath)
		if err != nil {
			log.Printf("Error reading %s: %v", filePath, err)
			return nil
		}
		files = append(files, FileData{Path: filePath, LineCount: lines})
		return nil
	})
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("error walking the path %q: %w", opts.Root, err)
	}

	var excludedFiles []FileData
	if opts.DropLargest > 0 {
		sort.Slice(files, func(i, j int) bool { return files[i].LineCount < files[j].LineCount })
		if opts.DropLargest >= len(files) {
			excludedFiles = files
			files = nil
		} else {
			excludedFiles = append(excludedFiles, files[len(files)-opts.DropLargest:]...)
			files = files[:len(files)-opts.DropLargest]
		}
	}

	if len(files) == 0 {
		return AnalysisResult{ExcludedFiles: excludedFiles}, nil
	}

	result := buildResult(files, excludedFiles, opts.ShowClusters)
	if opts.ListFiles {
		result.Files = sortedFiles(files, opts.SortMode)
	}
	return result, nil
}

func shouldAnalyze(relPath string, opts Options, config *Config) bool {
	relPath = normalizeRelPath(relPath)
	include := config.Include
	exclude := append([]string{}, config.Exclude...)
	exclude = append(exclude, opts.Exclude...)

	for _, pattern := range exclude {
		if matchPattern(pattern, relPath) {
			return false
		}
	}
	if opts.AllFiles {
		return true
	}

	if len(opts.Include) > 0 {
		include = opts.Include
	}
	if len(include) == 0 {
		return true
	}
	for _, pattern := range include {
		if matchPattern(pattern, relPath) {
			return true
		}
	}
	return false
}

func matchPattern(patternValue, relPath string) bool {
	patternValue = normalizeRelPath(patternValue)
	relPath = normalizeRelPath(relPath)
	base := path.Base(relPath)

	if strings.HasPrefix(patternValue, "**/") {
		suffix := strings.TrimPrefix(patternValue, "**/")
		if matched, _ := path.Match(suffix, base); matched {
			return true
		}
		if strings.HasSuffix(suffix, "/**") {
			dir := strings.TrimSuffix(suffix, "/**")
			return relPath == dir || strings.Contains(relPath, "/"+dir+"/")
		}
		return relPath == suffix || strings.HasSuffix(relPath, "/"+suffix)
	}
	if strings.HasSuffix(patternValue, "/**") {
		prefix := strings.TrimSuffix(patternValue, "/**")
		return relPath == prefix || strings.HasPrefix(relPath, prefix+"/")
	}
	if !strings.Contains(patternValue, "/") {
		matched, _ := path.Match(patternValue, base)
		if matched {
			return true
		}
		return base == patternValue
	}
	matched, _ := path.Match(patternValue, relPath)
	if matched {
		return true
	}
	return relPath == patternValue
}

func normalizeRelPath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "./")
	return strings.ToLower(value)
}

func countLines(filePath string) (int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	count := 0
	for {
		line, err := reader.ReadString('\n')
		if err == nil {
			count++
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(line) > 0 {
				count++
			}
			break
		}
		return count, err
	}
	return count, nil
}

func sortedFiles(files []FileData, sortMode string) []FileData {
	output := make([]FileData, len(files))
	copy(output, files)
	switch sortMode {
	case "path":
		sort.Slice(output, func(i, j int) bool { return output[i].Path < output[j].Path })
	case "lines":
		sort.Slice(output, func(i, j int) bool { return output[i].LineCount < output[j].LineCount })
	}
	return output
}

func loadGitignore(root string) (gitignoreRules, error) {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read .gitignore: %w", err)
	}
	return parseGitignore(string(data)), nil
}

type gitignoreRules []gitignoreRule

func parseGitignore(contents string) gitignoreRules {
	var rules gitignoreRules
	for _, raw := range strings.Split(contents, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := gitignoreRule{}
		if strings.HasPrefix(line, "!") {
			rule.Negate = true
			line = strings.TrimPrefix(line, "!")
		}
		line = strings.TrimPrefix(filepath.ToSlash(line), "./")
		if line == "" {
			continue
		}
		rule.Pattern = line
		rules = append(rules, rule)
	}
	return rules
}

func (rules gitignoreRules) ignores(relPath string, isDir bool) bool {
	ignored := false
	for _, rule := range rules {
		if gitignoreMatch(rule.Pattern, relPath, isDir) {
			ignored = !rule.Negate
		}
	}
	return ignored
}

func gitignoreMatch(patternValue, relPath string, isDir bool) bool {
	patternValue = normalizeRelPath(patternValue)
	relPath = normalizeRelPath(relPath)
	dirOnly := strings.HasSuffix(patternValue, "/")
	patternValue = strings.TrimSuffix(patternValue, "/")
	if dirOnly && !isDir && !strings.Contains(relPath, patternValue+"/") {
		return false
	}
	if !strings.Contains(patternValue, "/") {
		if isDir && relPath == patternValue {
			return true
		}
		for _, part := range strings.Split(relPath, "/") {
			if matched, _ := path.Match(patternValue, part); matched {
				return true
			}
		}
		return false
	}
	if dirOnly {
		return relPath == patternValue || strings.HasPrefix(relPath, patternValue+"/") || strings.Contains(relPath, "/"+patternValue+"/")
	}
	return matchPattern(patternValue, relPath)
}
