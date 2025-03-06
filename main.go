package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	Extensions []string `json:"extensions"`
	Exclusions []string `json:"exclusions"`
}

type FileData struct {
	Path      string
	LineCount int
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var config Config
	err = json.NewDecoder(f).Decode(&config)
	return &config, err
}

func countLines(filePath string) (int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

func computeStats(sizes []int) (avg float64, median float64, stdHigh float64, stdLow float64) {
	if len(sizes) == 0 {
		return 0, 0, 0, 0
	}

	var sum int
	for _, s := range sizes {
		sum += s
	}
	avg = float64(sum) / float64(len(sizes))

	// Sorting for median
	sorted := make([]int, len(sizes))
	copy(sorted, sizes)
	sort.Ints(sorted)
	n := len(sorted)
	if n%2 == 0 {
		median = float64(sorted[n/2-1]+sorted[n/2]) / 2.0
	} else {
		median = float64(sorted[n/2])
	}

	// Compute standard deviations separately for files above and below average
	var varianceHighSum float64
	var countHigh int
	var varianceLowSum float64
	var countLow int
	for _, s := range sizes {
		diff := float64(s) - avg
		if diff >= 0 {
			varianceHighSum += diff * diff
			countHigh++
		} else {
			varianceLowSum += diff * diff
			countLow++
		}
	}
	if countHigh > 0 {
		stdHigh = math.Sqrt(varianceHighSum / float64(countHigh))
	}
	if countLow > 0 {
		stdLow = math.Sqrt(varianceLowSum / float64(countLow))
	}
	return
}

func main() {
	// Define flags
	detailed := flag.Bool("l", false, "detailed output")
	sorted := flag.Bool("s", false, "detailed sorted output (smallest to largest)")
	histogram := flag.Bool("h", false, "detailed histogram output (graphical)")
	allFiles := flag.Bool("a", false, "all files, not just source code")
	helpFlag := flag.Bool("?", false, "print help")
	configPath := flag.String("config", "languages.json", "config file with file extensions")
	jsonOutput := flag.Bool("j", false, "save output as JSON to file")
	includeLang := flag.String("i", "", "include only this language type (extension)")
	excludeLang := flag.String("e", "", "exclude this language type (extension)")
	flag.Parse()

	// If help flag is provided, print usage and exit
	if *helpFlag {
		flag.Usage()
		return
	}

	// Get the folder argument
	if flag.NArg() < 1 {
		log.Fatal("Please provide a folder name")
	}
	root := flag.Arg(0)

	// Process language filtering flags
	allowed := make(map[string]bool)
	if *includeLang != "" {
		inc := strings.ToLower(*includeLang)
		if !strings.HasPrefix(inc, ".") {
			inc = "." + inc
		}
		allowed[inc] = true
	} else {
		// load configuration
		config, err := loadConfig(*configPath)
		if err != nil {
			log.Fatalf("Unable to load config: %v", err)
		}
		// Add allowed extensions from config.Extensions
		for _, ext := range config.Extensions {
			allowed[strings.ToLower(ext)] = true
		}
		// Remove exclusions from the allowed list
		for _, exc := range config.Exclusions {
			allowed[strings.ToLower(exc)] = false // or simply delete below
			delete(allowed, strings.ToLower(exc))
		}
		// Process exclude flag only if include flag is not specified
		if *excludeLang != "" {
			exc := strings.ToLower(*excludeLang)
			if !strings.HasPrefix(exc, ".") {
				exc = "." + exc
			}
			delete(allowed, exc)
		}
	}

	var files []FileData
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !*allFiles {
			lowerName := strings.ToLower(d.Name())
			match := false
			for ext := range allowed {
				if strings.HasSuffix(lowerName, ext) {
					match = true
					break
				}
			}
			if !match {
				return nil
			}
		}
		lines, err := countLines(path)
		if err != nil {
			log.Printf("Error reading %s: %v", path, err)
			return nil
		}
		files = append(files, FileData{Path: path, LineCount: lines})
		return nil
	}); err != nil {
		log.Fatalf("Error walking the path %q: %v", root, err)
	}

	if len(files) == 0 {
		fmt.Println("No files found")
		return
	}

	// Compute overall stats
	sizes := make([]int, len(files))
	smallest := files[0]
	largest := files[0]
	for i, fd := range files {
		sizes[i] = fd.LineCount
		if fd.LineCount < smallest.LineCount {
			smallest = fd
		}
		if fd.LineCount > largest.LineCount {
			largest = fd
		}
	}
	avg, median, stdHigh, stdLow := computeStats(sizes)

	// Compute sum total lines (only for non-all-files mode)
	sumTotal := 0
	for _, count := range sizes {
		sumTotal += count
	}

	// JSON output handling
	if *jsonOutput {
		output := struct {
			TotalFiles   int        `json:"total_files"`
			Average      float64    `json:"average"`
			Median       float64    `json:"median"`
			StdDevHigh   float64    `json:"std_dev_high"`
			StdDevLow    float64    `json:"std_dev_low"`
			TotalSum     *int       `json:"total_sum,omitempty"`
			SmallestFile FileData   `json:"smallest_file"`
			LargestFile  FileData   `json:"largest_file"`
			Files        []FileData `json:"files,omitempty"`
		}{
			TotalFiles:   len(files),
			Average:      avg,
			Median:       median,
			StdDevHigh:   stdHigh,
			StdDevLow:    stdLow,
			SmallestFile: smallest,
			LargestFile:  largest,
		}
		if !*allFiles {
			output.TotalSum = &sumTotal
		}
		// Add file list only if detailed or sorted (ignore histogram)
		if *detailed || *sorted {
			var jsonFiles []FileData
			if *sorted {
				jsonFiles = make([]FileData, len(files))
				copy(jsonFiles, files)
				sort.Slice(jsonFiles, func(i, j int) bool {
					return jsonFiles[i].LineCount < jsonFiles[j].LineCount
				})
			} else {
				jsonFiles = files
			}
			output.Files = jsonFiles
		}
		folderName := filepath.Base(root)
		jsonFileName := folderName + ".codesiz.json"
		f, err := os.Create(jsonFileName)
		if err != nil {
			log.Fatalf("Unable to create JSON file: %v", err)
		}
		defer f.Close()
		encoder := json.NewEncoder(f)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(output); err != nil {
			log.Fatalf("Error encoding JSON: %v", err)
		}
		fmt.Printf("JSON output saved to %s\n", jsonFileName)
		return
	}

	// Display results
	fmt.Printf("Total files analyzed: %d\n", len(files))
	fmt.Printf("Average lines: %.2f\n", avg)
	fmt.Printf("Median lines: %.2f\n", median)
	fmt.Printf("Standard deviation (high): %.2f\n", stdHigh)
	fmt.Printf("Standard deviation (low): %.2f\n", stdLow)
	// Show sum total only when -a is not specified.
	if !*allFiles {
		fmt.Printf("Total sum of lines: %d\n", sumTotal)
	}
	fmt.Printf("Smallest file: %s (%d lines)\n", smallest.Path, smallest.LineCount)
	fmt.Printf("Largest file: %s (%d lines)\n", largest.Path, largest.LineCount)

	// Print detailed file list according to flags
	if *histogram {
		const barWidth = 50
		maxLine := largest.LineCount
		var outputFiles []FileData
		if *sorted {
			outputFiles = make([]FileData, len(files))
			copy(outputFiles, files)
			sort.Slice(outputFiles, func(i, j int) bool {
				return outputFiles[i].LineCount < outputFiles[j].LineCount
			})
		} else {
			outputFiles = files
		}
		// Compute max width for file paths for alignment
		maxPathLen := 0
		for _, fd := range outputFiles {
			if len(fd.Path) > maxPathLen {
				maxPathLen = len(fd.Path)
			}
		}
		fmt.Println("\nDetailed file histogram:")
		for _, fd := range outputFiles {
			barLen := 0
			if maxLine > 0 {
				barLen = int((float64(fd.LineCount) / float64(maxLine)) * barWidth)
			}
			bar := strings.Repeat("█", barLen)
			fmt.Printf("%-*s: %s\n", maxPathLen, fd.Path, bar)
		}
	} else if *sorted {
		sortedFiles := make([]FileData, len(files))
		copy(sortedFiles, files)
		sort.Slice(sortedFiles, func(i, j int) bool {
			return sortedFiles[i].LineCount < sortedFiles[j].LineCount
		})
		fmt.Println("\nDetailed file list (sorted smallest to largest):")
		for _, fd := range sortedFiles {
			fmt.Printf("%s: %d lines\n", fd.Path, fd.LineCount)
		}
	} else if *detailed {
		fmt.Println("\nDetailed file list:")
		for _, fd := range files {
			fmt.Printf("%s: %d lines\n", fd.Path, fd.LineCount)
		}
	}
}
