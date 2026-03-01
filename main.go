// FrameScope is a macOS CPU profiler that measures per-process CPU consumption
// across configurable fixed-length time windows ("frames"). Unlike point-in-time
// monitors it shows how many CPU-seconds each process actually consumed during a
// window, making it easy to identify heavy consumers without watching a live graph.
//
// The application is built with Go and a native AppKit UI embedded via cgo. The
// Cocoa layer lives in cocoa_bridge.m and calls back into Go through the exported
// functions in controls.go. All shared mutable state is in the global monitorState
// struct (model.go), protected by a sync.Mutex.
package main

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "cocoa_bridge.h"
*/
import "C"

import (
	"runtime"
	"unsafe"
)

func main() {
	// Load persisted settings before the UI initialises so toolbar controls
	// reflect the saved values from the first draw.
	initializeConfig()

	// Push the build version into the Cocoa layer so it can be shown in the
	// window title before RunApp() starts the event loop.
	cVersion := C.CString(version)
	C.SetAppVersion(cVersion)
	C.free(unsafe.Pointer(cVersion))

	// The Cocoa run loop must execute on the main OS thread.
	runtime.LockOSThread()
	C.RunApp()
}
