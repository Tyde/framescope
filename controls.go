package main

/*
#include "cocoa_bridge.h"
*/
import "C"

import (
	"context"
	"fmt"
)

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
