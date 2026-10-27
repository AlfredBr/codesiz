package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
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

func loadConfigFromBytes(data []byte) (*Config, error) {
	var config *Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("profile must be a JSON object")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("profile must contain exactly one JSON object")
	}
	config.normalize()
	if len(config.Include) == 0 {
		return nil, fmt.Errorf("profile must contain at least one include pattern; use --all to scan all file types")
	}
	return config, nil
}

func loadConfig(filePath string) (*Config, error) {
	if filePath == "" {
		return loadConfigFromBytes(embeddedConfig)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read profile %q: %w", filePath, err)
	}
	config, err := loadConfigFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parse profile %q: %w", filePath, err)
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
