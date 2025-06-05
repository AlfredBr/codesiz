//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/magefile/mage/sh"
)

// Default target to run when none is specified
var Default = Build

// Build compiles the Go application
func Build() error {
	fmt.Println("Building codesiz...")

	// Determine the output filename based on OS
	outputName := "codesiz"
	if runtime.GOOS == "windows" {
		outputName = "codesiz.exe"
	}

	// Build the application
	return sh.Run("go", "build", "-o", outputName, "main.go")
}

// Clean removes build artifacts and temporary files
func Clean() error {
	fmt.Println("Cleaning up build artifacts and temporary files...")

	filesToClean := []string{
		"codesiz",
		"codesiz.exe",
		"main.exe",
		"*.codesiz.json", // JSON output files created by the application
	}

	for _, pattern := range filesToClean {
		if pattern == "*.codesiz.json" {
			// Handle glob pattern for JSON files
			matches, err := filepath.Glob(pattern)
			if err != nil {
				fmt.Printf("Error finding files matching %s: %v\n", pattern, err)
				continue
			}
			for _, match := range matches {
				if err := os.Remove(match); err != nil {
					fmt.Printf("Warning: could not remove %s: %v\n", match, err)
				} else {
					fmt.Printf("Removed: %s\n", match)
				}
			}
		} else {
			// Handle individual files
			if _, err := os.Stat(pattern); err == nil {
				if err := os.Remove(pattern); err != nil {
					fmt.Printf("Warning: could not remove %s: %v\n", pattern, err)
				} else {
					fmt.Printf("Removed: %s\n", pattern)
				}
			}
		}
	}

	fmt.Println("Clean completed")
	return nil
}

// Test runs the Go tests (if any exist)
func Test() error {
	fmt.Println("Running tests...")
	return sh.Run("go", "test", "./...")
}

// Install installs the application to GOPATH/bin
func Install() error {
	fmt.Println("Installing codesiz...")
	return sh.Run("go", "install")
}

// Mod tidies up the go.mod file
func Mod() error {
	fmt.Println("Tidying go.mod...")
	return sh.Run("go", "mod", "tidy")
}
