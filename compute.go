package main

import "sort"

// computeResults diffs two process snapshots and returns one resultRow per
// process that was present in both. Processes that exited between the two
// snapshots (absent from current) are omitted. Negative diffs — which can
// occur when a PID is reused by a new process mid-frame — are also discarded.
//
// The returned slice is sorted by CPU consumption descending, with PID as a
// tiebreaker for a stable ordering.
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
