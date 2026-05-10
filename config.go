package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed languages.json
var embeddedConfig []byte

type Config struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func loadConfigFromBytes(data []byte) (*Config, error) {
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	config.normalize()
	return &config, nil
}

func loadConfig(filePath string) (*Config, error) {
	if filePath != "languages.json" && fileExists(filePath) {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open custom config file %s: %w", filePath, err)
		}
		defer f.Close()
		var config Config
		if err := json.NewDecoder(f).Decode(&config); err != nil {
			return nil, fmt.Errorf("failed to parse custom config file %s: %w", filePath, err)
		}
		config.normalize()
		return &config, nil
	}

	if fileExists("languages.json") {
		f, err := os.Open("languages.json")
		if err != nil {
			return nil, fmt.Errorf("failed to open default config file: %w", err)
		}
		defer f.Close()
		var config Config
		if err := json.NewDecoder(f).Decode(&config); err != nil {
			return nil, fmt.Errorf("failed to parse default config file: %w", err)
		}
		config.normalize()
		return &config, nil
	}

	config, err := loadConfigFromBytes(embeddedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded config: %w", err)
	}
	return config, nil
}

func (c *Config) normalize() {
	c.Include = normalizePatterns(c.Include)
	c.Exclude = normalizePatterns(c.Exclude)
}

func normalizePatterns(patterns []string) []string {
	seen := map[string]bool{}
	var normalized []string
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		pattern = strings.TrimPrefix(pattern, "./")
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		normalized = append(normalized, pattern)
	}
	return normalized
}
