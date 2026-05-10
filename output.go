package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

func writeResult(stdout io.Writer, opts Options, result AnalysisResult) error {
	if opts.Format == "json" {
		var writer io.Writer = stdout
		var f *os.File
		if opts.OutputPath != "" {
			var err error
			f, err = os.Create(opts.OutputPath)
			if err != nil {
				return fmt.Errorf("unable to create JSON file: %w", err)
			}
			defer f.Close()
			writer = f
		}
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("error encoding JSON: %w", err)
		}
		if opts.OutputPath != "" {
			fmt.Fprintf(stdout, "JSON output saved to %s\n", opts.OutputPath)
		}
		return nil
	}

	if result.TotalFiles == 0 {
		if len(result.ExcludedFiles) > 0 {
			fmt.Fprintf(stdout, "No files left after excluding %d file(s)\n", len(result.ExcludedFiles))
		} else {
			fmt.Fprintln(stdout, "No files found")
		}
		return nil
	}

	if len(result.ExcludedFiles) > 0 {
		fmt.Fprintf(stdout, "Excluded %d largest file(s):\n", len(result.ExcludedFiles))
		for _, fd := range result.ExcludedFiles {
			fmt.Fprintf(stdout, " - %s: %d lines\n", fd.Path, fd.LineCount)
		}
		fmt.Fprintln(stdout)
	}

	if opts.Format == "histogram" {
		writeHistogram(stdout, result.Files)
		return nil
	}

	fmt.Fprintf(stdout, "Total files analyzed: %d\n", result.TotalFiles)
	fmt.Fprintf(stdout, "Total lines: %d\n", result.TotalSum)
	fmt.Fprintf(stdout, "Average: %.0f lines\n", math.Round(result.Average))
	fmt.Fprintf(stdout, "Median: %.0f lines\n", math.Round(result.Median))
	fmt.Fprintf(stdout, "Standard deviation (high): %.0f lines\n", math.Round(result.StdDevHigh))
	fmt.Fprintf(stdout, "Standard deviation (low): %.0f lines\n", math.Round(result.StdDevLow))
	fmt.Fprintf(stdout, "Smallest file: %s (%d lines)\n", result.SmallestFile.Path, result.SmallestFile.LineCount)
	fmt.Fprintf(stdout, "Largest file: %s (%d lines)\n", result.LargestFile.Path, result.LargestFile.LineCount)

	if opts.ShowClusters && len(result.Clusters) > 0 {
		fmt.Fprintln(stdout, "\nFile clusters (k-means clustering, k=3):")
		for _, cluster := range result.Clusters {
			fmt.Fprintf(stdout, " %s: %d files (%.2f%%), Avg = %.0f lines, Range = [%.0f, %.0f] lines\n",
				cluster.Label, cluster.Count, cluster.Percentage, cluster.Avg, cluster.Range[0], cluster.Range[1])
		}
	}

	if opts.ListFiles {
		fmt.Fprintln(stdout, "\nDetailed file list:")
		for _, fd := range result.Files {
			fmt.Fprintf(stdout, "%s: %d lines\n", fd.Path, fd.LineCount)
		}
	}
	return nil
}

func writeHistogram(stdout io.Writer, files []FileData) {
	const barWidth = 50
	maxLine := 0
	maxPathLen := 0
	for _, fd := range files {
		if fd.LineCount > maxLine {
			maxLine = fd.LineCount
		}
		if len(fd.Path) > maxPathLen {
			maxPathLen = len(fd.Path)
		}
	}
	fmt.Fprintln(stdout, "Detailed file histogram:")
	for _, fd := range files {
		barLen := 0
		if maxLine > 0 {
			barLen = int((float64(fd.LineCount) / float64(maxLine)) * barWidth)
		}
		bar := strings.Repeat("#", barLen)
		fmt.Fprintf(stdout, "%-*s: %s %d\n", maxPathLen, fd.Path, bar, fd.LineCount)
	}
}
