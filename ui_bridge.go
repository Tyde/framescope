package main

/*
#include <stdlib.h>
#include "cocoa_bridge.h"
*/
import "C"

import "unsafe"

// pushUI snapshots the current application state (under the mutex), renders
// the table and summary payloads, and calls postUpdate to deliver them to the
// Cocoa layer on the main thread. Passing runID = 0 bypasses the stale-run
// check and always delivers the update (used after user-initiated actions such
// as Stop or frame selection).
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

// postUpdate passes rendered string payloads to the Cocoa UpdateResults
// function via cgo. Each Go string is copied into a C string, passed to
// Cocoa (which dispatches to the main queue asynchronously), and then freed
// immediately. The call is a no-op if runID refers to a stale monitoring run.
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

// postError passes an error message string to the Cocoa ShowErrorMessage
// function. The message replaces the status bar text and clears both tables.
// The call is a no-op if runID refers to a stale monitoring run.
func postError(runID int64, message string) {
	if !isCurrentRun(runID) {
		return
	}
	cMessage := C.CString(message)
	C.ShowErrorMessage(cMessage)
	C.free(unsafe.Pointer(cMessage))
}

// isCurrentRun reports whether runID still matches the active monitoring run.
// A runID of 0 is a sentinel meaning "always current", used for UI updates
// that are not tied to a specific monitoring session.
func isCurrentRun(runID int64) bool {
	if runID == 0 {
		return true
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.runID == runID
}
