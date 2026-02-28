package main

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa
#include "cocoa_bridge.h"
*/
import "C"

import "runtime"

func main() {
	initializeConfig()
	runtime.LockOSThread()
	C.RunApp()
}
