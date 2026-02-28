package main

import "fmt"

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

func cloneRows(rows []resultRow) []resultRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]resultRow, len(rows))
	copy(out, rows)
	return out
}

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
