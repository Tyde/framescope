package main

/*
#include <stdlib.h>
#include "cocoa_bridge.h"
*/
import "C"

import "unsafe"

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
