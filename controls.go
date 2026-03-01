package main

/*
#include "cocoa_bridge.h"
*/
import "C"

import (
	"context"
	"fmt"
)

// GoStartMonitoring is called from Cocoa when the user presses Start. It
// cancels any in-progress monitoring run, resets all frame state, and launches
// a new runMonitor goroutine. frameSeconds is the desired frame length; values
// ≤ 0 are rejected with an error message.
//
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
	state.frameSeconds = interval
	state.frameIndex = 1
	state.history = nil
	state.liveRows = nil
	state.selectedHistoryIdx = -1
	state.viewingCurrent = true
	state.autoFollowLatestComplete = true
	state.status = fmt.Sprintf("Running. Frame 1 of %.1fs started.", interval)
	state.mu.Unlock()
	saveConfig()

	go runMonitor(ctx, runID, interval)
	pushUI(runID)
}

// GoStopMonitoring is called from Cocoa when the user presses Stop. It
// cancels the active monitoring goroutine and updates state so the UI shows
// the last completed frame.
//
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

// GoSetHideSmall is called from Cocoa when the user toggles the "Hide <1s"
// option. enabled is non-zero for on, zero for off. The new setting is
// persisted to disk immediately.
//
//export GoSetHideSmall
func GoSetHideSmall(enabled C.int) {
	state.mu.Lock()
	state.hideSmall = enabled != 0
	state.mu.Unlock()
	saveConfig()
	pushUI(0)
}

// GoSetHidePaths is called from Cocoa when the user toggles the "Basename
// only" option. enabled is non-zero for on, zero for off. The new setting is
// persisted to disk immediately.
//
//export GoSetHidePaths
func GoSetHidePaths(enabled C.int) {
	state.mu.Lock()
	state.hidePaths = enabled != 0
	state.mu.Unlock()
	saveConfig()
	pushUI(0)
}

// GoSelectFrame is called from Cocoa when the user picks an entry from the
// history popup or clicks Prev / Next. selectedIndex is the popup item index;
// it maps to either a completed frame in history or the live in-progress frame
// (when monitoring is running and the last item is selected). Out-of-range
// indices are ignored.
//
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

// GoInitialHideSmall is called from Cocoa during startup to read the persisted
// hideSmall preference so the toolbar checkbox can be initialised correctly.
// Returns 1 if enabled, 0 otherwise.
//
//export GoInitialHideSmall
func GoInitialHideSmall() C.int {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.hideSmall {
		return 1
	}
	return 0
}

// GoInitialHidePaths is called from Cocoa during startup to read the persisted
// hidePaths preference so the toolbar checkbox can be initialised correctly.
// Returns 1 if enabled, 0 otherwise.
//
//export GoInitialHidePaths
func GoInitialHidePaths() C.int {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.hidePaths {
		return 1
	}
	return 0
}

// GoInitialFrameSeconds is called from Cocoa during startup to populate the
// frame-length text field with the persisted value.
//
//export GoInitialFrameSeconds
func GoInitialFrameSeconds() C.double {
	state.mu.Lock()
	defer state.mu.Unlock()
	return C.double(state.frameSeconds)
}
