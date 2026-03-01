package main

import "fmt"

// currentViewLabelLocked returns a short human-readable label describing which
// frame the UI is currently showing. Must be called with state.mu held.
func currentViewLabelLocked() string {
	if state.viewingCurrent {
		if state.running {
			return fmt.Sprintf("Current Frame %d", state.frameIndex)
		}
		return "Current Frame"
	}
	if state.selectedHistoryIdx >= 0 && state.selectedHistoryIdx < len(state.history) {
		return fmt.Sprintf("Frame %d", state.history[state.selectedHistoryIdx].Index)
	}
	return "Latest Frame"
}

// currentRowsLocked returns a copy of the rows that should be displayed in the
// main table. It resolves the view priority:
//  1. Live in-progress frame, if viewingCurrent is set.
//  2. The explicitly selected history entry.
//  3. The most recently completed frame as a fallback.
//
// Must be called with state.mu held.
func currentRowsLocked() []resultRow {
	if state.viewingCurrent {
		return cloneRows(state.liveRows)
	}
	if state.selectedHistoryIdx >= 0 && state.selectedHistoryIdx < len(state.history) {
		return cloneRows(state.history[state.selectedHistoryIdx].Rows)
	}
	if len(state.history) > 0 {
		return cloneRows(state.history[len(state.history)-1].Rows)
	}
	return cloneRows(state.liveRows)
}

// historyPayloadLocked builds the newline-separated list of frame labels sent
// to the Cocoa history popup, and returns the index of the currently selected
// item (-1 if none). Must be called with state.mu held.
func historyPayloadLocked() (string, int) {
	items := make([]string, 0, len(state.history)+1)
	selected := -1

	for i, frame := range state.history {
		items = append(items, fmt.Sprintf("Frame %d", frame.Index))
		if !state.viewingCurrent && state.selectedHistoryIdx == i {
			selected = i
		}
	}

	if state.running {
		items = append(items, fmt.Sprintf("Current Frame %d (in progress)", state.frameIndex))
		if state.viewingCurrent {
			selected = len(items) - 1
		}
	}

	return joinLines(items), selected
}

// cloneRows returns a shallow copy of rows so callers can safely release
// state.mu before using the slice.
func cloneRows(rows []resultRow) []resultRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]resultRow, len(rows))
	copy(out, rows)
	return out
}

// joinLines joins a slice of strings with newline separators. Returns an empty
// string for an empty slice.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	result := lines[0]
	for _, line := range lines[1:] {
		result += "\n" + line
	}
	return result
}
