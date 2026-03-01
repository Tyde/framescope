package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// buildStatusLocked composes the status-bar string shown while monitoring is
// active. It reports the current frame number, configured length, elapsed and
// remaining time within the frame, the number of visible rows, and which frame
// the user is viewing. Must be called with state.mu held.
func buildStatusLocked(frameSeconds float64, frameStart, now time.Time, rows []resultRow) string {
	frameIndex := state.frameIndex
	hideSmall := state.hideSmall
	viewLabel := currentViewLabelLocked()

	elapsed := now.Sub(frameStart).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	remaining := frameSeconds - elapsed
	if remaining < 0 {
		remaining = 0
	}

	visibleRows := len(rows)
	if hideSmall {
		visibleRows = 0
		for _, row := range rows {
			if row.Diff >= 1 {
				visibleRows++
			}
		}
	}

	return fmt.Sprintf(
		"Running. Frame %d | length %.1fs | elapsed %.1fs | remaining %.1fs | %d visible processes | viewing %s",
		frameIndex,
		frameSeconds,
		elapsed,
		remaining,
		visibleRows,
		viewLabel,
	)
}

// renderTable converts a slice of result rows into the tab-separated text
// payload consumed by the Cocoa table view. Each line contains:
//
//	PID \t CPU-seconds \t HH:MM:SS \t command
//
// Rows below 1 CPU-second are filtered out when hideSmall is true. Output is
// capped at 500 rows to keep the UI responsive. Tabs and newlines in command
// strings are replaced by spaces via sanitizeCommand.
func renderTable(rows []resultRow, hideSmall, hidePaths bool) string {
	filtered := make([]resultRow, 0, len(rows))
	for _, row := range rows {
		if hideSmall && row.Diff < 1 {
			continue
		}
		filtered = append(filtered, row)
	}

	var b strings.Builder
	limit := len(filtered)
	if limit > 500 {
		limit = 500
	}

	for i := 0; i < limit; i++ {
		row := filtered[i]
		command := sanitizeCommand(row.Command, hidePaths)
		fmt.Fprintf(&b, "%d\t%.1f\t%s\t%s\n", row.PID, row.Diff, formatDuration(row.Diff), command)
	}

	return b.String()
}

// renderSummaryTable aggregates CPU usage across all completed frames and
// returns a tab-separated payload for the summary table view. Each line
// contains:
//
//	PID \t total-s \t avg-s \t total-HH:MM:SS \t avg-HH:MM:SS \t command
//
// Averages are computed over the total number of completed frames (not just
// the frames in which a process appeared). Output is capped at 500 rows.
// Returns an empty string if no frames have completed yet.
func renderSummaryTable(history []frameRecord, hideSmall, hidePaths bool) string {
	frameCount := len(history)
	if frameCount == 0 {
		return ""
	}

	type aggregateState struct {
		total   float64
		command string
	}

	aggregates := make(map[int]aggregateState)
	for _, frame := range history {
		for _, row := range frame.Rows {
			entry := aggregates[row.PID]
			entry.total += row.Diff
			if entry.command == "" {
				entry.command = row.Command
			}
			aggregates[row.PID] = entry
		}
	}

	rows := make([]aggregateRow, 0, len(aggregates))
	for pid, entry := range aggregates {
		avg := entry.total / float64(frameCount)
		if hideSmall && entry.total < 1 {
			continue
		}
		rows = append(rows, aggregateRow{
			PID:     pid,
			Total:   entry.total,
			Average: avg,
			Command: entry.command,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Total == rows[j].Total {
			return rows[i].PID < rows[j].PID
		}
		return rows[i].Total > rows[j].Total
	})

	var b strings.Builder
	limit := len(rows)
	if limit > 500 {
		limit = 500
	}

	for i := 0; i < limit; i++ {
		row := rows[i]
		command := sanitizeCommand(row.Command, hidePaths)
		fmt.Fprintf(
			&b,
			"%d\t%.1f\t%.1f\t%s\t%s\t%s\n",
			row.PID,
			row.Total,
			row.Average,
			formatDuration(row.Total),
			formatDuration(row.Average),
			command,
		)
	}

	return b.String()
}

// sanitizeCommand prepares a raw command string for display. If hidePaths is
// true only the basename of the executable is kept (arguments are dropped).
// Tabs and newlines are replaced with spaces to preserve the integrity of the
// tab-separated table format.
func sanitizeCommand(command string, hidePaths bool) string {
	if hidePaths {
		command = baseCommand(command)
	}
	command = strings.ReplaceAll(command, "\t", " ")
	command = strings.ReplaceAll(command, "\n", " ")
	return command
}

// baseCommand extracts the basename of the first whitespace-delimited token in
// command (i.e. the executable path), discarding any arguments. Returns the
// original string unchanged if it contains no fields.
func baseCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return command
	}
	parts := strings.Split(fields[0], "/")
	return parts[len(parts)-1]
}

// formatDuration formats a duration expressed as fractional seconds into the
// human-readable HH:MM:SS string used in both table views.
func formatDuration(seconds float64) string {
	total := int(seconds)
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}
