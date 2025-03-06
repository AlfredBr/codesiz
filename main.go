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

// Types and Helper Functions
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
	// Compute average and median
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

// runKMeans performs k-means clustering on float64 data with fixed iterations.
func runKMeans(data []float64, k int) (assignments []int, centroids []float64) {
	n := len(data)
	// Initialize centroids using min, median and max of sorted data.
	sortedData := make([]float64, n)
	copy(sortedData, data)
	sort.Float64s(sortedData)
	centroids = []float64{sortedData[0], sortedData[n/2], sortedData[n-1]}
	assignments = make([]int, n)
	for iter := 0; iter < 10; iter++ {
		changed := false
		for i, x := range data {
			best := 0
			bestDist := math.Abs(x - centroids[0])
			for j := 1; j < k; j++ {
				d := math.Abs(x - centroids[j])
				if d < bestDist {
					bestDist = d
					best = j
				}
			}
			if assignments[i] != best {
				assignments[i] = best
				changed = true
			}
		}
		newCentroids := make([]float64, k)
		counts := make([]int, k)
		for i, cluster := range assignments {
			newCentroids[cluster] += data[i]
			counts[cluster]++
		}
		for j := 0; j < k; j++ {
			if counts[j] > 0 {
				newCentroids[j] /= float64(counts[j])
			} else {
				newCentroids[j] = centroids[j]
			}
		}
		centroids = newCentroids
		if !changed {
			break
		}
	}
	return
}

type clusterSummary struct {
	Cluster int
	Count   int
	Sum     float64
	Min     float64
	Max     float64
	Avg     float64
}

// computeClusterSummaries computes summaries from the data and cluster assignments.
func computeClusterSummaries(data []float64, assignments []int, k int) []clusterSummary {
	summaries := make([]clusterSummary, k)
	for j := 0; j < k; j++ {
		summaries[j].Min = 1e9
		summaries[j].Max = -1
		summaries[j].Cluster = j
	}
	for i, cluster := range assignments {
		x := data[i]
		summaries[cluster].Count++
		summaries[cluster].Sum += x
		if x < summaries[cluster].Min {
			summaries[cluster].Min = x
		}
		if x > summaries[cluster].Max {
			summaries[cluster].Max = x
		}
	}
	for j := 0; j < k; j++ {
		if summaries[j].Count > 0 {
			summaries[j].Avg = summaries[j].Sum / float64(summaries[j].Count)
		}
	}
	return summaries
}

// labelClusters returns a map from cluster index to label based on average value.
func labelClusters(summaries []clusterSummary) map[int]string {
	k := len(summaries)
	type idxAvg struct {
		Index int
		Avg   float64
	}
	idxs := make([]idxAvg, k)
	for j := 0; j < k; j++ {
		idxs[j] = idxAvg{Index: j, Avg: summaries[j].Avg}
	}
	sort.Slice(idxs, func(i, j int) bool { return idxs[i].Avg < idxs[j].Avg })
	labels := map[int]string{
		idxs[0].Index: "Small",
		idxs[1].Index: "Medium",
		idxs[2].Index: "Large",
	}
	return labels
}

// ----- Main Function -----
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
	kFlag := flag.Int("k", 0, "exclude largest n files") // NEW - added new flag
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

	// Remove allowed map logic and load config only when needed.
	var config *Config
	if *includeLang == "" {
		var err error
		config, err = loadConfig(*configPath)
		if err != nil {
			log.Fatalf("Unable to load config: %v", err)
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
			// Check exclude flag if provided.
			if *excludeLang != "" {
				exc := strings.ToLower(*excludeLang)
				if !strings.HasPrefix(exc, ".") {
					exc = "." + exc
				}
				if strings.HasSuffix(lowerName, exc) {
					return nil
				}
			}
			if *includeLang != "" {
				inc := strings.ToLower(*includeLang)
				if !strings.HasPrefix(inc, ".") {
					inc = "." + inc
				}
				if !strings.HasSuffix(lowerName, inc) {
					return nil
				}
			} else {
				// First, exclude files with any exclusion extension.
				for _, exc := range config.Exclusions {
					if strings.HasSuffix(lowerName, strings.ToLower(exc)) {
						return nil
					}
				}
				// Then, check if file name matches any allowed extension.
				allowFlag := false
				for _, ext := range config.Extensions {
					if strings.HasSuffix(lowerName, strings.ToLower(ext)) {
						allowFlag = true
						break
					}
				}
				if !allowFlag {
					return nil
				}
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

	// Exclude the largest n files if -k is provided.
	var excludedFiles []FileData
	if *kFlag > 0 {
		// sort files smallest to largest so that largest files are at the end
		sort.Slice(files, func(i, j int) bool { return files[i].LineCount < files[j].LineCount })
		if *kFlag >= len(files) {
			fmt.Printf("Excluding %d file(s) (largest files).\n", len(files))
			excludedFiles = files
			files = []FileData{}
		} else {
			n := *kFlag
			excludedFiles = files[len(files)-n:]
			files = files[:len(files)-n]
			fmt.Printf("Excluding %d largest file(s):\n", len(excludedFiles))
			for _, fd := range excludedFiles {
				// Print each excluded file with its line count.
				fmt.Printf(" - %s: %d lines\n", fd.Path, fd.LineCount)
			}
			fmt.Println()
		}
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

	// Compute clusters once using k-means if possible.
	var clusterResults []struct {
		Label      string     `json:"label"`
		Count      int        `json:"count"`
		Percentage float64    `json:"percentage"`
		Avg        float64    `json:"avg"`
		Range      [2]float64 `json:"range"`
	}
	if len(files) >= 3 {
		k := 3
		n := len(files)
		data := make([]float64, n)
		for i, fd := range files {
			data[i] = float64(fd.LineCount)
		}
		assignments, _ := runKMeans(data, k)
		summaries := computeClusterSummaries(data, assignments, k)
		labels := labelClusters(summaries)
		clusterResults = make([]struct {
			Label      string     `json:"label"`
			Count      int        `json:"count"`
			Percentage float64    `json:"percentage"`
			Avg        float64    `json:"avg"`
			Range      [2]float64 `json:"range"`
		}, k)
		for j := 0; j < k; j++ {
			avgVal := 0.0
			if summaries[j].Count > 0 {
				avgVal = summaries[j].Sum / float64(summaries[j].Count)
			}
			perc := 100.0 * float64(summaries[j].Count) / float64(n)
			clusterResults[j] = struct {
				Label      string     `json:"label"`
				Count      int        `json:"count"`
				Percentage float64    `json:"percentage"`
				Avg        float64    `json:"avg"`
				Range      [2]float64 `json:"range"`
			}{
				Label:      labels[j],
				Count:      summaries[j].Count,
				Percentage: perc,
				Avg:        math.Round(avgVal),
				Range:      [2]float64{math.Round(summaries[j].Min), math.Round(summaries[j].Max)},
			}
		}
	}

	// JSON Output section.
	if *jsonOutput {
		output := struct {
			TotalFiles   int         `json:"total_files"`
			Average      float64     `json:"average"`
			Median       float64     `json:"median"`
			StdDevHigh   float64     `json:"std_dev_high"`
			StdDevLow    float64     `json:"std_dev_low"`
			TotalSum     *int        `json:"total_sum,omitempty"`
			SmallestFile FileData    `json:"smallest_file"`
			LargestFile  FileData    `json:"largest_file"`
			Files        []FileData  `json:"files,omitempty"`
			Clusters     interface{} `json:"clusters,omitempty"`
		}{
			TotalFiles:   len(files),
			Average:      avg,
			Median:       median,
			StdDevHigh:   stdHigh,
			StdDevLow:    stdLow,
			SmallestFile: smallest,
			LargestFile:  largest,
			TotalSum: func() *int {
				if !*allFiles {
					return &sumTotal
				}
				return nil
			}(),
			Files: func() []FileData {
				if *detailed || *sorted {
					jsonFiles := make([]FileData, len(files))
					copy(jsonFiles, files)
					if *sorted {
						sort.Slice(jsonFiles, func(i, j int) bool {
							return jsonFiles[i].LineCount < jsonFiles[j].LineCount
						})
					}
					return jsonFiles
				}
				return nil
			}(),
			Clusters: clusterResults,
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

	// Display results (updated to round all line counts)
	fmt.Printf("Total files analyzed: %d\n", len(files))
	fmt.Printf("Average: %.0f lines\n", math.Round(avg))
	fmt.Printf("Median: %.0f lines\n", math.Round(median))
	fmt.Printf("Standard deviation (high): %.0f lines\n", math.Round(stdHigh))
	fmt.Printf("Standard deviation (low): %.0f lines\n", math.Round(stdLow))
	if !*allFiles {
		fmt.Printf("Total sum: %d lines\n", sumTotal)
	}
	fmt.Printf("Smallest file: %s (%d lines)\n", smallest.Path, smallest.LineCount)
	fmt.Printf("Largest file: %s (%d lines)\n", largest.Path, largest.LineCount)

	// Compute file clusters using k-means clustering (k=3) on file line counts.
	if len(files) >= 3 {
		fmt.Println("\nFile clusters (k-means clustering, k=3):")
		for j := 0; j < 3; j++ {
			perc := 100.0 * float64(clusterResults[j].Count) / float64(len(files))
			fmt.Printf(" %s: %d files (%.2f%%), Avg = %.0f lines, Range = [%.0f, %.0f] lines\n",
				clusterResults[j].Label, clusterResults[j].Count, perc,
				clusterResults[j].Avg, clusterResults[j].Range[0], clusterResults[j].Range[1])
		}
	} else {
		fmt.Println("\nNot enough files for clustering.")
	}

	// Detailed file listing (histogram/sorted/detailed)
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
