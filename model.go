// FrameScope is a macOS CPU profiler that measures per-process CPU consumption
// across configurable fixed-length time windows ("frames"). It samples cumulative
// CPU time at the start and end of each frame and reports the difference, making
// it easy to see which processes were most active over a given period.
package main

import (
	"context"
	"sync"
)

// processSample holds a single process's cumulative CPU usage at a point in time,
// captured during a snapshot. Both User and System CPU seconds are summed into
// CPUSeconds.
type processSample struct {
	CPUSeconds float64 // total user+system CPU seconds consumed so far
	Command    string  // full command line, or name if cmdline is unavailable
}

// resultRow is a computed row in the results table, representing the CPU
// consumed by one process between two snapshots (one frame interval).
type resultRow struct {
	PID     int
	Diff    float64 // CPU-seconds consumed during the frame
	Command string
}

// frameRecord stores the completed results for a single frame, identified by
// its sequential frame number.
type frameRecord struct {
	Index int         // 1-based frame number assigned when the frame completed
	Rows  []resultRow // results sorted by CPU consumption (descending)
}

// aggregateRow represents a process's totals and per-frame averages across all
// completed frames, used to populate the summary table.
type aggregateRow struct {
	PID     int
	Total   float64 // sum of CPU-seconds across all frames the process appeared in
	Average float64 // Total / number of completed frames
	Command string
}

// monitorState is the single shared mutable state for the application.
// All fields must be accessed with mu held, except where noted.
type monitorState struct {
	mu           sync.Mutex
	running      bool    // true while a monitoring goroutine is active
	hideSmall    bool    // filter rows below 1 CPU-second in the UI
	hidePaths    bool    // show only basename of the command, not full path
	frameSeconds float64 // configured frame length in seconds
	frameIndex   int     // 1-based index of the frame currently being collected

	// runID is incremented each time monitoring starts or stops. It is used by
	// background goroutines to detect whether their results are still relevant.
	runID  int64
	cancel context.CancelFunc // cancels the active monitoring context; nil when stopped

	// history holds completed frames, capped at maxHistory entries (oldest dropped).
	history []frameRecord

	// liveRows holds the latest computed rows for the frame currently in progress.
	// Nil when no frame is active.
	liveRows []resultRow

	// selectedHistoryIdx is the index into history that the user is viewing.
	// -1 means no specific frame is selected (defers to viewingCurrent).
	selectedHistoryIdx int

	// viewingCurrent is true when the UI is displaying the live in-progress frame
	// rather than a completed one from history.
	viewingCurrent bool

	// autoFollowLatestComplete causes the UI to automatically advance to the
	// newest completed frame whenever a frame finishes, unless the user has
	// manually navigated away.
	autoFollowLatestComplete bool

	status string // human-readable status line shown in the status bar
}

// state is the single global application state, initialised with sensible defaults.
var state = &monitorState{
	hideSmall:    true,
	hidePaths:    false,
	frameSeconds: 15,
}
