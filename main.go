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

type monitorState struct {
	mu         sync.Mutex
	running    bool
	hideSmall  bool
	hidePaths  bool
	frameIndex int
	runID      int64
	cancel     context.CancelFunc
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
	state.mu.Unlock()

	go runMonitor(ctx, runID, interval)
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

	postUpdate(0, "Monitoring stopped.", "Press Start to begin a new measurement loop.")
}

//export GoSetHideSmall
func GoSetHideSmall(enabled C.int) {
	state.mu.Lock()
	state.hideSmall = enabled != 0
	state.mu.Unlock()
}

//export GoSetHidePaths
func GoSetHidePaths(enabled C.int) {
	state.mu.Lock()
	state.hidePaths = enabled != 0
	state.mu.Unlock()
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

	postUpdate(
		runID,
		fmt.Sprintf("Running. Frame 1 of %.1fs started.", frameSeconds),
		"Collecting initial samples...",
	)

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
			status := buildStatus(frameSeconds, frameStart, now, results)
			postUpdate(runID, status, renderTable(results))

			if now.Sub(frameStart) >= frameDuration {
				state.mu.Lock()
				if !state.running {
					state.mu.Unlock()
					return
				}
				state.frameIndex++
				frameIndex := state.frameIndex
				state.mu.Unlock()

				baseline = current
				frameStart = now
				postUpdate(
					runID,
					fmt.Sprintf("Frame %d started. Length %.1fs.", frameIndex, frameSeconds),
					renderTable(nil),
				)
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

func buildStatus(frameSeconds float64, frameStart, now time.Time, rows []resultRow) string {
	state.mu.Lock()
	frameIndex := state.frameIndex
	hideSmall := state.hideSmall
	state.mu.Unlock()

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
		"Running. Frame %d | length %.1fs | elapsed %.1fs | remaining %.1fs | %d visible processes",
		frameIndex,
		frameSeconds,
		elapsed,
		remaining,
		visibleRows,
	)
}

func renderTable(rows []resultRow) string {
	state.mu.Lock()
	hideSmall := state.hideSmall
	hidePaths := state.hidePaths
	state.mu.Unlock()

	filtered := make([]resultRow, 0, len(rows))
	for _, row := range rows {
		if hideSmall && row.Diff < 1 {
			continue
		}
		filtered = append(filtered, row)
	}

	if len(filtered) == 0 {
		return " PID      Raw(s)   CPU Time     Command\n\n No processes match the current filters yet."
	}

	var b strings.Builder
	b.WriteString(" PID      Raw(s)   CPU Time     Command\n")
	b.WriteString(" -----------------------------------------------")

	limit := len(filtered)
	if limit > 100 {
		limit = 100
	}

	for i := 0; i < limit; i++ {
		row := filtered[i]
		command := row.Command
		if hidePaths {
			command = baseCommand(command)
		}
		command = truncate(command, 100)

		fmt.Fprintf(
			&b,
			"\n %-8d %-8.1f %-12s %s",
			row.PID,
			row.Diff,
			formatDuration(row.Diff),
			command,
		)
	}

	if len(filtered) > limit {
		fmt.Fprintf(&b, "\n\n Showing top %d of %d rows.", limit, len(filtered))
	}

	return b.String()
}

func baseCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return command
	}
	parts := strings.Split(fields[0], "/")
	return parts[len(parts)-1]
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

func formatDuration(seconds float64) string {
	total := int(seconds)
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}

func postUpdate(runID int64, status, table string) {
	if !isCurrentRun(runID) {
		return
	}
	cStatus := C.CString(status)
	cTable := C.CString(table)
	C.UpdateResults(cStatus, cTable)
	C.free(unsafe.Pointer(cStatus))
	C.free(unsafe.Pointer(cTable))
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
