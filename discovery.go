package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type discoveredType struct {
	Name        string
	Selected    int
	Filtered    int
	Ignored     int
	NotIncluded int
	Reasons     map[string]int
}

// Discovery inventories names only; it never guesses whether unknown content is source.
func discover(opts Options, config *Config) ([]discoveredType, []string, error) {
	if err := validateRoot(opts.Root); err != nil {
		return nil, nil, err
	}
	ignores, err := loadGitignore(opts.Root)
	if err != nil {
		return nil, nil, err
	}
	groups := map[string]*discoveredType{}
	patterns := map[string]bool{}
	err = filepath.WalkDir(opts.Root, func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(opts.Root, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(name))
		label := ext
		if label == "" {
			label = "(no extension)"
		}
		group := groups[label]
		if group == nil {
			group = &discoveredType{Name: label, Reasons: map[string]int{}}
			groups[label] = group
		}
		ignored := ignores.ignores(rel, false)
		// Analysis prunes ignored parents, so children cannot re-include themselves.
		for parent := filepath.ToSlash(filepath.Dir(rel)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if ignores.ignores(parent, true) {
				ignored = true
			}
		}
		if ignored {
			group.Ignored++
		} else if !shouldAnalyze(rel, opts, config) {
			group.Filtered++
			reason := "no include pattern matched"
			for _, pattern := range append(append([]string{}, config.Exclude...), opts.Exclude...) {
				if matchPattern(pattern, rel) {
					reason = "excluded by " + pattern
					break
				}
			}
			group.Reasons[reason]++
			if reason == "no include pattern matched" {
				group.NotIncluded++
			}
		} else {
			group.Selected++
			if ext != "" {
				patterns["**/*"+escapeGlob(ext)] = true
			} else {
				patterns[escapeGlob(filepath.Base(name))] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("discover %q: %w", opts.Root, err)
	}
	rows := make([]discoveredType, 0, len(groups))
	for _, row := range groups {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	includes := make([]string, 0, len(patterns))
	for pattern := range patterns {
		includes = append(includes, pattern)
	}
	sort.Strings(includes)
	return rows, includes, nil
}

func escapeGlob(value string) string {
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "?", "\\?", "[", "\\[", "]", "\\]").Replace(value)
}

func validateRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("open scan folder %q: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("scan path %q must be a directory", root)
	}
	return nil
}

func runDiscovery(out io.Writer, opts Options, config *Config) error {
	rows, includes, err := discover(opts, config)
	if err != nil {
		return err
	}
	source := "built-in"
	if opts.ConfigPath != "" {
		source = opts.ConfigPath
	}
	var report strings.Builder
	fmt.Fprintf(&report, "File types in %s | Profile: %s\n", opts.Root, source)
	fmt.Fprintln(&report, "Type                 Selected   Filtered   Git-ignored")
	for _, row := range rows {
		fmt.Fprintf(&report, "%-20s %8d %10d %13d\n", row.Name, row.Selected, row.Filtered, row.Ignored)
		reasons := make([]string, 0, len(row.Reasons))
		for reason := range row.Reasons {
			reasons = append(reasons, reason)
		}
		sort.Strings(reasons)
		for _, reason := range reasons {
			fmt.Fprintf(&report, "  %d: %s\n", row.Reasons[reason], reason)
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(&report, "No regular files found.")
	}
	fmt.Fprintln(&report, "Filtered files do not match includes or match an exclusion. Git-ignored uses the root .gitignore.")
	fmt.Fprintln(&report, "Unknown types are not automatically included. Review them with --include, then repeat --discover or --init.")
	for _, row := range rows {
		if row.NotIncluded == 0 || row.Name == "(no extension)" {
			continue
		}
		selection := "the profile includes"
		if len(opts.Include) > 0 {
			selection = "your --include patterns"
		} else if opts.ConfigPath == "" {
			selection = "the default profile"
		}
		fmt.Fprintf(&report, "Found %d %s file(s) excluded by %s. Review whether they belong in this analysis.\n", row.NotIncluded, row.Name, selection)
		// --include replaces profile includes, so keep the current selection in the suggested command.
		current := config.Include
		if len(opts.Include) > 0 {
			current = opts.Include
		}
		fmt.Fprint(&report, "  Preview with: codesiz --discover")
		if opts.ConfigPath != "" {
			fmt.Fprintf(&report, " --config %q", opts.ConfigPath)
		}
		for _, pattern := range normalizePatterns(append(append([]string{}, current...), "**/*"+escapeGlob(row.Name))) {
			fmt.Fprintf(&report, " --include %q", pattern)
		}
		for _, pattern := range opts.Exclude {
			fmt.Fprintf(&report, " --exclude %q", pattern)
		}
		fmt.Fprintf(&report, " %q\n", opts.Root)
	}
	if _, err := io.WriteString(out, report.String()); err != nil {
		return err
	}
	if !opts.Init {
		return nil
	}
	// Preserve explicit selection boundaries rather than broaden custom path rules.
	if opts.ConfigPath != "" {
		includes = config.Include
	}
	if len(opts.Include) > 0 {
		includes = opts.Include
	}
	if len(includes) == 0 {
		return fmt.Errorf("no selected file types; use --include to select files before --init")
	}
	profile := Config{Include: includes, Exclude: append(append([]string{}, config.Exclude...), opts.Exclude...)}
	profile.normalize()
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	destination := opts.OutputPath
	if destination == "" {
		destination = filepath.Join(opts.Root, "codesiz.json")
	}
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create profile %q (existing files are never overwritten): %w", destination, err)
	}
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("write profile %q: %w", destination, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close profile %q: %w", destination, closeErr)
	}
	_, err = fmt.Fprintf(out, "Profile saved to %s. Review it, then run codesiz --config %q %q\n", destination, destination, opts.Root)
	return err
}
