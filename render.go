package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

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

func sanitizeCommand(command string, hidePaths bool) string {
	if hidePaths {
		command = baseCommand(command)
	}
	command = strings.ReplaceAll(command, "\t", " ")
	command = strings.ReplaceAll(command, "\n", " ")
	return command
}

func baseCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return command
	}
	parts := strings.Split(fields[0], "/")
	return parts[len(parts)-1]
}

func formatDuration(seconds float64) string {
	total := int(seconds)
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}
