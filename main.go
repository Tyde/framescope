package main

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "cocoa_bridge.h"
*/
import "C"

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/v3/process"
)

type processSample struct {
	CPUSeconds float64
	Command    string
}

type resultRow struct {
	PID     int
	Diff    float64
	Command string
}

type frameRecord struct {
	Index int
	Rows  []resultRow
}

type aggregateRow struct {
	PID     int
	Total   float64
	Average float64
	Command string
}

type monitorState struct {
	mu                       sync.Mutex
	running                  bool
	hideSmall                bool
	hidePaths                bool
	frameIndex               int
	runID                    int64
	cancel                   context.CancelFunc
	history                  []frameRecord
	liveRows                 []resultRow
	selectedHistoryIdx       int
	viewingCurrent           bool
	autoFollowLatestComplete bool
	status                   string
}

var state = &monitorState{
	hideSmall: true,
	hidePaths: false,
}

func main() {
	runtime.LockOSThread()
	C.RunApp()
}

//export GoStartMonitoring
func GoStartMonitoring(frameSeconds C.double) {
	interval := float64(frameSeconds)
	if interval <= 0 {
		postError(0, "Frame length must be greater than zero seconds.")
		return
	}

	state.mu.Lock()
	if state.cancel != nil {
		state.cancel()
		state.cancel = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.runID++
	runID := state.runID
	state.cancel = cancel
	state.running = true
	state.frameIndex = 1
	state.history = nil
	state.liveRows = nil
	state.selectedHistoryIdx = -1
	state.viewingCurrent = true
	state.autoFollowLatestComplete = true
	state.status = fmt.Sprintf("Running. Frame 1 of %.1fs started.", interval)
	state.mu.Unlock()

	go runMonitor(ctx, runID, interval)
	pushUI(runID)
}

//export GoStopMonitoring
func GoStopMonitoring() {
	state.mu.Lock()
	cancel := state.cancel
	state.runID++
	state.cancel = nil
	state.running = false
	state.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	state.mu.Lock()
	state.status = "Monitoring stopped."
	if len(state.history) > 0 && state.autoFollowLatestComplete {
		state.viewingCurrent = false
		state.selectedHistoryIdx = len(state.history) - 1
	}
	state.mu.Unlock()

	pushUI(0)
}

//export GoSetHideSmall
func GoSetHideSmall(enabled C.int) {
	state.mu.Lock()
	state.hideSmall = enabled != 0
	state.mu.Unlock()
	pushUI(0)
}

//export GoSetHidePaths
func GoSetHidePaths(enabled C.int) {
	state.mu.Lock()
	state.hidePaths = enabled != 0
	state.mu.Unlock()
	pushUI(0)
}

//export GoSelectFrame
func GoSelectFrame(selectedIndex C.int) {
	state.mu.Lock()
	index := int(selectedIndex)
	completedCount := len(state.history)
	currentIndex := -1
	if state.running {
		currentIndex = completedCount
	}

	switch {
	case index == currentIndex:
		state.viewingCurrent = true
		state.selectedHistoryIdx = -1
		state.autoFollowLatestComplete = false
	case index >= 0 && index < completedCount:
		state.viewingCurrent = false
		state.selectedHistoryIdx = index
		state.autoFollowLatestComplete = index == completedCount-1
	default:
		state.mu.Unlock()
		return
	}
	state.mu.Unlock()
	pushUI(0)
}

func runMonitor(ctx context.Context, runID int64, frameSeconds float64) {
	baseline, err := snapshot()
	if err != nil {
		postError(runID, fmt.Sprintf("Initial snapshot failed: %v", err))
		stopFromWorker(runID)
		return
	}

	frameDuration := time.Duration(frameSeconds * float64(time.Second))
	frameStart := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			current, err := snapshot()
			if err != nil {
				postError(runID, fmt.Sprintf("Snapshot failed: %v", err))
				continue
			}

			results := computeResults(baseline, current)
			state.mu.Lock()
			state.liveRows = cloneRows(results)
			state.status = buildStatusLocked(frameSeconds, frameStart, now, results)
			state.mu.Unlock()
			pushUI(runID)

			if now.Sub(frameStart) >= frameDuration {
				state.mu.Lock()
				if !state.running {
					state.mu.Unlock()
					return
				}
				completedFrameIndex := state.frameIndex
				state.history = append(state.history, frameRecord{
					Index: completedFrameIndex,
					Rows:  cloneRows(results),
				})
				if state.autoFollowLatestComplete || len(state.history) == 1 {
					state.viewingCurrent = false
					state.selectedHistoryIdx = len(state.history) - 1
					state.autoFollowLatestComplete = true
				}
				state.frameIndex++
				frameIndex := state.frameIndex
				state.liveRows = nil
				state.status = fmt.Sprintf("Running. Frame %d started. Length %.1fs.", frameIndex, frameSeconds)
				state.mu.Unlock()

				baseline = current
				frameStart = now
				pushUI(runID)
			}
		}
	}
}

func stopFromWorker(runID int64) {
	state.mu.Lock()
	if state.runID != runID {
		state.mu.Unlock()
		return
	}
	state.running = false
	state.cancel = nil
	state.mu.Unlock()
}

func snapshot() (map[int]processSample, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	results := make(map[int]processSample, len(processes))
	for _, proc := range processes {
		if proc == nil || proc.Pid <= 0 {
			continue
		}

		times, err := proc.Times()
		if err != nil {
			continue
		}

		command, err := proc.Cmdline()
		if err != nil || command == "" {
			command, err = proc.Name()
		}
		if command == "" {
			command = "<unknown>"
		}

		results[int(proc.Pid)] = processSample{
			CPUSeconds: times.User + times.System,
			Command:    command,
		}
	}

	return results, nil
}

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
		command := row.Command
		if hidePaths {
			command = baseCommand(command)
		}
		command = strings.ReplaceAll(command, "\t", " ")
		command = strings.ReplaceAll(command, "\n", " ")
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
		command := row.Command
		if hidePaths {
			command = baseCommand(command)
		}
		command = strings.ReplaceAll(command, "\t", " ")
		command = strings.ReplaceAll(command, "\n", " ")
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

func cloneRows(rows []resultRow) []resultRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]resultRow, len(rows))
	copy(out, rows)
	return out
}

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

	return strings.Join(items, "\n"), selected
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

func pushUI(runID int64) {
	state.mu.Lock()
	status := state.status
	hideSmall := state.hideSmall
	hidePaths := state.hidePaths
	rows := currentRowsLocked()
	history := append([]frameRecord(nil), state.history...)
	historyText, selectedIndex := historyPayloadLocked()
	state.mu.Unlock()
	table := renderTable(rows, hideSmall, hidePaths)
	summary := renderSummaryTable(history, hideSmall, hidePaths)
	postUpdate(runID, status, table, summary, historyText, selectedIndex)
}

func postUpdate(runID int64, status, table, summary, historyText string, selectedIndex int) {
	if !isCurrentRun(runID) {
		return
	}
	cStatus := C.CString(status)
	cTable := C.CString(table)
	cSummary := C.CString(summary)
	cHistory := C.CString(historyText)
	C.UpdateResults(cStatus, cTable, cSummary, cHistory, C.int(selectedIndex))
	C.free(unsafe.Pointer(cStatus))
	C.free(unsafe.Pointer(cTable))
	C.free(unsafe.Pointer(cSummary))
	C.free(unsafe.Pointer(cHistory))
}

func postError(runID int64, message string) {
	if !isCurrentRun(runID) {
		return
	}
	cMessage := C.CString(message)
	C.ShowErrorMessage(cMessage)
	C.free(unsafe.Pointer(cMessage))
}

func isCurrentRun(runID int64) bool {
	if runID == 0 {
		return true
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.runID == runID
}
