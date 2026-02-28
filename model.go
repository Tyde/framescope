package main

import (
	"context"
	"sync"
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
