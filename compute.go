package main

import "sort"

func computeResults(initial, current map[int]processSample) []resultRow {
	rows := make([]resultRow, 0, len(initial))
	for pid, before := range initial {
		after, ok := current[pid]
		if !ok {
			continue
		}

		diff := after.CPUSeconds - before.CPUSeconds
		if diff < 0 {
			continue
		}

		rows = append(rows, resultRow{
			PID:     pid,
			Diff:    diff,
			Command: before.Command,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Diff == rows[j].Diff {
			return rows[i].PID < rows[j].PID
		}
		return rows[i].Diff > rows[j].Diff
	})

	return rows
}
