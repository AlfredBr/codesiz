package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltInProfileIgnoresLocalConfig(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "languages.json", "not json")
	t.Chdir(dir)
	config, err := loadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if !shouldAnalyze("main.go", Options{}, config) {
		t.Fatal("built-in source selection missing")
	}
	if _, err := loadConfig("languages.json"); err == nil {
		t.Fatal("explicit invalid config should fail")
	}
	if _, err := loadConfig("missing.json"); err == nil {
		t.Fatal("explicit missing config should fail")
	}
}

func TestProfileValidation(t *testing.T) {
	for _, input := range []string{`null`, `{}`, `{"include":[]}`, `{"include":["*.go"],"typo":true}`, `{"include":["*.go"]} {}`} {
		if _, err := loadConfigFromBytes([]byte(input)); err == nil {
			t.Errorf("accepted invalid profile %s", input)
		}
	}
}

func TestDiscoveryAndGeneratedProfile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "main.go", "one\n")
	writeTestFile(t, dir, "query.sql", "select 1;\n")
	writeTestFile(t, dir, "ignored.go", "one\n")
	writeTestFile(t, dir, ".gitignore", "ignored.go\n")
	writeTestFile(t, filepath.Join(dir, "vendor"), "noise.go", "one\n")
	config, err := loadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Root: dir, Init: true}
	rows, _, err := discover(opts, config)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.Name == ".go" {
			found = true
			if row.Selected != 1 || row.Filtered != 1 || row.Ignored != 1 {
				t.Fatalf("incorrect counts: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("missing Go inventory")
	}
	var out bytes.Buffer
	if err := runDiscovery(&out, opts, config); err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{
		"Found 1 .sql file(s) excluded by the default profile",
		"no include pattern matched",
		"excluded by vendor/**",
		`--include "**/*.go"`,
		`--include "**/*.sql"`,
	} {
		if !strings.Contains(out.String(), message) {
			t.Errorf("discovery missing %q: %s", message, out.String())
		}
	}
	if strings.Contains(out.String(), "Found 1 .go file(s)") {
		t.Fatal("suggested overriding an explicit exclusion")
	}
	target := filepath.Join(dir, "codesiz.json")
	generated, err := loadConfig(target)
	if err != nil {
		t.Fatal(err)
	}
	if !shouldAnalyze("future.go", Options{}, generated) || shouldAnalyze("query.sql", Options{}, generated) || shouldAnalyze("vendor/noise.go", Options{}, generated) {
		t.Fatalf("unexpected profile: %+v", generated)
	}
	before, _ := os.ReadFile(target)
	if err := runDiscovery(&out, opts, config); err == nil {
		t.Fatal("overwrote existing profile")
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("existing profile changed")
	}
}

func TestDiscoverySuggestionsPreserveOverrides(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "query.sql", "select 1")
	writeTestFile(t, dir, "image.png", "binary")
	writeTestFile(t, dir, "ignored.py", "print(1)")
	writeTestFile(t, dir, ".gitignore", "ignored.py\n")
	config := &Config{Include: []string{"src/*.go"}, Exclude: []string{"**/*.png"}}
	opts := Options{Root: dir, Discover: true, ConfigPath: "team.json", Include: []string{"src/*.ts"}, Exclude: []string{"private/**"}}
	var out bytes.Buffer
	if err := runDiscovery(&out, opts, config); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`--config "team.json"`, `--include "src/*.ts"`, `--include "**/*.sql"`, `--exclude "private/**"`, "excluded by your --include patterns"} {
		if !strings.Contains(out.String(), text) {
			t.Errorf("missing %q in %s", text, out.String())
		}
	}
	if strings.Contains(out.String(), `--include "**/*.png"`) || strings.Contains(out.String(), `--include "**/*.py"`) || strings.Contains(out.String(), `--include "src/*.go"`) {
		t.Fatalf("suggested ignored/excluded types or replaced overrides: %s", out.String())
	}
	if shouldAnalyze("query.sql", opts, config) {
		t.Fatal("discovery changed selection")
	}
}

func TestInitPreservesExplicitSelectionAndEmptyScan(t *testing.T) {
	dir := t.TempDir()
	config, _ := loadConfig("")
	var out bytes.Buffer
	opts := Options{Root: dir, Init: true}
	if err := runDiscovery(&out, opts, config); err == nil {
		t.Fatal("empty discovery must not generate allow-all profile")
	}
	opts.Include = []string{"src/*.sql"}
	if err := runDiscovery(&out, opts, config); err != nil {
		t.Fatal(err)
	}
	profile, err := loadConfig(filepath.Join(dir, "codesiz.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(profile.Include, ",") != "src/*.sql" {
		t.Fatalf("custom boundary changed: %+v", profile)
	}
}

func TestDiscoveryOptionsAndRootValidation(t *testing.T) {
	opts, err := parseOptions(nil)
	if err != nil || opts.Root != "." || opts.ConfigPath != "" {
		t.Fatalf("defaults: %+v, %v", opts, err)
	}
	for _, args := range [][]string{{"--discover", "--all"}, {"--init", "--format", "json"}, {"--discover", "--output", "out.json"}} {
		if _, err := parseOptions(args); err == nil {
			t.Errorf("accepted incompatible options %v", args)
		}
	}
	dir := t.TempDir()
	writeTestFile(t, dir, "file.go", "one")
	if _, _, err := discover(Options{Root: filepath.Join(dir, "file.go")}, &Config{}); err == nil {
		t.Fatal("file root accepted")
	}
}
