package main

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

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
