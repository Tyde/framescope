package main

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// runMonitor is the core sampling loop. It runs in its own goroutine and is
// cancelled via ctx when the user stops monitoring or starts a new run.
//
// The loop ticks every 500 ms. On each tick it takes a snapshot of all running
// processes, diffs the CPU times against the baseline, updates liveRows in the
// shared state, and pushes a UI refresh. When the elapsed time reaches
// frameSeconds the current snapshot becomes the baseline for the next frame, the
// completed frame is appended to history, and the cycle resets.
//
// runID is compared against state.runID on every write to detect stale goroutines
// from previous runs.
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

	// updateFrame takes a fresh snapshot, computes results, updates state, and
	// pushes a UI refresh. If the frame duration has elapsed it also finalises
	// the completed frame and resets the baseline.
	updateFrame := func(now time.Time) error {
		current, err := snapshot()
		if err != nil {
			return err
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
				return nil
			}
			// maxHistory caps the number of retained completed frames. When the
			// limit is exceeded the oldest entry is discarded and selectedHistoryIdx
			// is adjusted so the UI selection remains stable.
			const maxHistory = 1000
			completedFrameIndex := state.frameIndex
			state.history = append(state.history, frameRecord{
				Index: completedFrameIndex,
				Rows:  cloneRows(results),
			})
			if len(state.history) > maxHistory {
				state.history = state.history[1:]
				if state.selectedHistoryIdx > 0 {
					state.selectedHistoryIdx--
				}
			}
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

		return nil
	}

	// Take an immediate first snapshot so the UI is not blank for the first tick.
	if err := updateFrame(frameStart); err != nil {
		postError(runID, fmt.Sprintf("Snapshot failed: %v", err))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := updateFrame(now); err != nil {
				postError(runID, fmt.Sprintf("Snapshot failed: %v", err))
			}
		}
	}
}

// stopFromWorker marks monitoring as stopped in the global state. It is called
// by the monitor goroutine itself when it encounters a fatal error. It is a
// no-op if runID no longer matches state.runID (i.e. a newer run has already
// started).
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

// snapshot reads the current CPU times and command for every running process
// and returns them keyed by PID. Processes that cannot be queried (e.g. due to
// insufficient permissions) are silently skipped. A best-effort command string
// is derived by preferring the full command line and falling back to the process
// name.
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
