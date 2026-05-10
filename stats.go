package main

import (
	"fmt"
	"math"
	"sort"
)

type ClusterSummary struct {
	Cluster int
	Count   int
	Sum     float64
	Min     float64
	Max     float64
	Avg     float64
}

type ClusterResult struct {
	Label      string     `json:"label"`
	Count      int        `json:"count"`
	Percentage float64    `json:"percentage"`
	Avg        float64    `json:"avg"`
	Range      [2]float64 `json:"range"`
}

func buildResult(files []FileData, excludedFiles []FileData, showClusters bool) AnalysisResult {
	sizes := make([]int, len(files))
	smallest := files[0]
	largest := files[0]
	sumTotal := 0
	for i, fd := range files {
		sizes[i] = fd.LineCount
		sumTotal += fd.LineCount
		if fd.LineCount < smallest.LineCount {
			smallest = fd
		}
		if fd.LineCount > largest.LineCount {
			largest = fd
		}
	}

	avg, median, stdHigh, stdLow := computeStats(sizes)
	result := AnalysisResult{
		TotalFiles:    len(files),
		Average:       avg,
		Median:        median,
		StdDevHigh:    stdHigh,
		StdDevLow:     stdLow,
		TotalSum:      sumTotal,
		SmallestFile:  smallest,
		LargestFile:   largest,
		ExcludedFiles: excludedFiles,
	}
	if showClusters && len(files) >= 3 {
		result.Clusters = computeClusters(files, 3)
	}
	return result
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

	sorted := make([]int, len(sizes))
	copy(sorted, sizes)
	sort.Ints(sorted)
	n := len(sorted)
	if n%2 == 0 {
		median = float64(sorted[n/2-1]+sorted[n/2]) / 2.0
	} else {
		median = float64(sorted[n/2])
	}

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

func computeClusters(files []FileData, k int) []ClusterResult {
	data := make([]float64, len(files))
	for i, fd := range files {
		data[i] = float64(fd.LineCount)
	}
	assignments, _ := runKMeans(data, k)
	summaries := computeClusterSummaries(data, assignments, k)
	labels := labelClusters(summaries)
	results := make([]ClusterResult, k)
	for j := 0; j < k; j++ {
		perc := 100.0 * float64(summaries[j].Count) / float64(len(files))
		results[j] = ClusterResult{
			Label:      labels[j],
			Count:      summaries[j].Count,
			Percentage: perc,
			Avg:        math.Round(summaries[j].Avg),
			Range:      [2]float64{math.Round(summaries[j].Min), math.Round(summaries[j].Max)},
		}
	}
	return results
}

func runKMeans(data []float64, k int) (assignments []int, centroids []float64) {
	n := len(data)
	if n == 0 || k <= 0 {
		return nil, nil
	}
	if k > n {
		k = n
	}

	sortedData := make([]float64, n)
	copy(sortedData, data)
	sort.Float64s(sortedData)
	centroids = make([]float64, k)
	for i := 0; i < k; i++ {
		index := 0
		if k > 1 {
			index = int(math.Round(float64(i) * float64(n-1) / float64(k-1)))
		}
		centroids[i] = sortedData[index]
	}

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

func computeClusterSummaries(data []float64, assignments []int, k int) []ClusterSummary {
	summaries := make([]ClusterSummary, k)
	for j := 0; j < k; j++ {
		summaries[j].Min = math.Inf(1)
		summaries[j].Max = math.Inf(-1)
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
		} else {
			summaries[j].Min = 0
			summaries[j].Max = 0
		}
	}
	return summaries
}

func labelClusters(summaries []ClusterSummary) map[int]string {
	labelsBySize := []string{"Small", "Medium", "Large"}
	type idxAvg struct {
		Index int
		Avg   float64
	}
	idxs := make([]idxAvg, len(summaries))
	for j := range summaries {
		idxs[j] = idxAvg{Index: j, Avg: summaries[j].Avg}
	}
	sort.Slice(idxs, func(i, j int) bool { return idxs[i].Avg < idxs[j].Avg })
	labels := map[int]string{}
	for i, idx := range idxs {
		if i < len(labelsBySize) {
			labels[idx.Index] = labelsBySize[i]
		} else {
			labels[idx.Index] = fmt.Sprintf("Cluster %d", i+1)
		}
	}
	return labels
}
