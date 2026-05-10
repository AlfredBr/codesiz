package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestShouldAnalyzeUsesIncludeAndExcludePatterns(t *testing.T) {
	config := &Config{
		Include: []string{"**/*.go", "**/*.ts", "**/*.generated.*"},
		Exclude: []string{"vendor/**", "**/node_modules/**", "**/*.generated.*"},
	}
	config.normalize()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "included source", path: "cmd/app/main.go", want: true},
		{name: "unsupported extension", path: "README.md", want: false},
		{name: "root vendor excluded", path: "vendor/pkg/file.go", want: false},
		{name: "nested node modules excluded", path: "web/node_modules/pkg/index.ts", want: false},
		{name: "generated suffix excluded", path: "models/user.generated.ts", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAnalyze(tt.path, Options{}, config); got != tt.want {
				t.Fatalf("shouldAnalyze(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestShouldAnalyzeExplicitIncludeOverridesProfileIncludes(t *testing.T) {
	config := &Config{Include: []string{"**/*.go"}}
	config.normalize()
	opts := Options{Include: []string{"**/*.md"}}

	if shouldAnalyze("main.go", opts, config) {
		t.Fatal("main.go should not match when explicit include limits the scan to markdown")
	}
	if !shouldAnalyze("README.md", opts, config) {
		t.Fatal("README.md should match explicit include")
	}
}

func TestCountLinesHandlesLongLinesAndFinalLineWithoutNewline(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "long.txt")
	longLine := strings.Repeat("x", 128*1024)
	content := longLine + "\nsecond line\nthird line without newline"
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	count, err := countLines(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("countLines() = %d, want 3", count)
	}
}

func TestAnalyzeHonorsGitignoreByDefault(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".gitignore", "*.go\n!important.go\nvendor/\n")
	writeTestFile(t, dir, "main.go", "one\n")
	writeTestFile(t, dir, "ignored.go", "one\n")
	writeTestFile(t, dir, "important.go", "one\n")
	writeTestFile(t, filepath.Join(dir, "vendor"), "pkg.go", "one\n")

	result, err := analyze(Options{Root: dir, ListFiles: true, SortMode: "path", ShowClusters: true}, &Config{Include: []string{"**/*.go"}})
	if err != nil {
		t.Fatal(err)
	}

	got := baseNames(result.Files)
	want := []string{"important.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("files = %v, want %v", got, want)
	}
}

func TestAnalyzeDropLargest(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "small.go", "one\n")
	writeTestFile(t, dir, "medium.go", "one\ntwo\n")
	writeTestFile(t, dir, "large.go", "one\ntwo\nthree\n")

	result, err := analyze(Options{Root: dir, ListFiles: true, SortMode: "lines", DropLargest: 1, ShowClusters: true}, &Config{Include: []string{"**/*.go"}})
	if err != nil {
		t.Fatal(err)
	}

	if result.TotalFiles != 2 || result.TotalSum != 3 {
		t.Fatalf("result totals = files:%d sum:%d, want files:2 sum:3", result.TotalFiles, result.TotalSum)
	}
	if len(result.ExcludedFiles) != 1 || filepath.Base(result.ExcludedFiles[0].Path) != "large.go" {
		t.Fatalf("excluded files = %+v, want large.go", result.ExcludedFiles)
	}
	if result.LargestFile.LineCount != 2 {
		t.Fatalf("largest after drop = %d lines, want 2", result.LargestFile.LineCount)
	}
}

func TestComputeStats(t *testing.T) {
	avg, median, stdHigh, stdLow := computeStats([]int{10, 20, 30, 100})

	assertFloat(t, avg, 40)
	assertFloat(t, median, 25)
	assertFloat(t, stdHigh, 60)
	assertFloat(t, stdLow, math.Sqrt((30*30+20*20+10*10)/3.0))
}

func TestParseOptionsUsesModernFlags(t *testing.T) {
	opts, err := parseOptions([]string{
		"--format", "json",
		"--output", "report.json",
		"--list",
		"--sort", "lines",
		"--include", "**/*.go,**/*.md",
		"--exclude", "vendor/**",
		"--drop-largest", "2",
		"--no-clusters",
		".",
	})
	if err != nil {
		t.Fatal(err)
	}

	if opts.Format != "json" || opts.OutputPath != "report.json" || !opts.ListFiles || opts.SortMode != "lines" {
		t.Fatalf("unexpected output options: %+v", opts)
	}
	if opts.DropLargest != 2 {
		t.Fatalf("DropLargest = %d, want 2", opts.DropLargest)
	}
	if opts.ShowClusters {
		t.Fatal("ShowClusters = true, want false")
	}
	if got := strings.Join(opts.Include, ","); got != "**/*.go,**/*.md" {
		t.Fatalf("Include = %q", got)
	}
	if got := strings.Join(opts.Exclude, ","); got != "vendor/**" {
		t.Fatalf("Exclude = %q", got)
	}
}

func TestParseOptionsHelp(t *testing.T) {
	_, err := parseOptions([]string{"-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseOptions(-h) error = %v, want flag.ErrHelp", err)
	}
}

func TestWriteJSONDefaultsToStdout(t *testing.T) {
	result := AnalysisResult{
		TotalFiles:   1,
		TotalSum:     7,
		Average:      7,
		Median:       7,
		SmallestFile: FileData{Path: "main.go", LineCount: 7},
		LargestFile:  FileData{Path: "main.go", LineCount: 7},
	}
	var out bytes.Buffer
	if err := writeResult(&out, Options{Format: "json"}, result); err != nil {
		t.Fatal(err)
	}

	var decoded AnalysisResult
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if decoded.TotalFiles != 1 || decoded.TotalSum != 7 {
		t.Fatalf("decoded result = %+v", decoded)
	}
}

func TestWriteJSONToFile(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "report.json")
	result := AnalysisResult{
		TotalFiles:   1,
		TotalSum:     5,
		Average:      5,
		Median:       5,
		SmallestFile: FileData{Path: "main.go", LineCount: 5},
		LargestFile:  FileData{Path: "main.go", LineCount: 5},
	}

	var stdout bytes.Buffer
	if err := writeResult(&stdout, Options{Format: "json", OutputPath: outputPath}, result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "JSON output saved") {
		t.Fatalf("stdout = %q, want saved message", stdout.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AnalysisResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TotalFiles != 1 || decoded.TotalSum != 5 {
		t.Fatalf("decoded result = %+v", decoded)
	}
}

func TestWriteHistogramUsesSolidBars(t *testing.T) {
	result := AnalysisResult{
		TotalFiles: 2,
		Files: []FileData{
			{Path: "small.go", LineCount: 1},
			{Path: "middle.go", LineCount: 101},
			{Path: "large.go", LineCount: 202},
		},
	}

	var out bytes.Buffer
	if err := writeResult(&out, Options{Format: "histogram"}, result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "█") {
		t.Fatalf("histogram = %q, want solid bar", out.String())
	}
	if strings.Contains(out.String(), "#") {
		t.Fatalf("histogram = %q, should not use hash bars", out.String())
	}
	if !strings.Contains(out.String(), "█████████████████████████ 101") {
		t.Fatalf("histogram = %q, want half-width resolution", out.String())
	}
	if !strings.Contains(out.String(), "▎") {
		t.Fatalf("histogram = %q, want fractional bar glyph", out.String())
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func baseNames(files []FileData) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = filepath.Base(file.Path)
	}
	sort.Strings(names)
	return names
}

func assertFloat(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("got %f, want %f", got, want)
	}
}
