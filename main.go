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
	initializeConfig()
	cVersion := C.CString(version)
	C.SetAppVersion(cVersion)
	C.free(unsafe.Pointer(cVersion))
	runtime.LockOSThread()
	C.RunApp()
}
