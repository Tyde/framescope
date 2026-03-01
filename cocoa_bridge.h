/**
 * cocoa_bridge.h — C interface between Go (cgo) and the Cocoa UI layer.
 *
 * Functions prefixed with Go* are implemented in Go (controls.go) and exported
 * via cgo for use by Objective-C. All other functions are implemented in
 * cocoa_bridge.m and called from Go.
 *
 * All UI updates must reach AppKit on the main thread. Go callers invoke these
 * functions from arbitrary goroutines; implementations in cocoa_bridge.m are
 * responsible for dispatching to the main queue where required.
 */

#ifndef MONITOR_CPU_COCOA_BRIDGE_H
#define MONITOR_CPU_COCOA_BRIDGE_H

/**
 * SetAppVersion stores the build version string so it can be embedded in the
 * window title when the window is created. Must be called before RunApp().
 */
void SetAppVersion(const char *version);

/**
 * RunApp initialises NSApplication, installs MonitorAppDelegate as the app
 * delegate, and enters the Cocoa run loop. This function never returns.
 * The caller must have locked the OS thread (runtime.LockOSThread) before
 * calling.
 */
void RunApp(void);

/**
 * UpdateResults delivers a complete UI refresh to the main thread. All four
 * string parameters are tab/newline-separated payloads rendered by Go:
 *
 *   status      — plain-text status bar string
 *   tableText   — tab-separated rows for the current-frame table (4 columns)
 *   summaryText — tab-separated rows for the summary table (6 columns)
 *   historyText — newline-separated frame labels for the history popup
 *   selectedIndex — popup item index to select (-1 for none)
 *
 * The function dispatches asynchronously to the main queue; it is safe to
 * call from any goroutine.
 */
void UpdateResults(const char *status, const char *tableText,
                   const char *summaryText, const char *historyText,
                   int selectedIndex);

/**
 * ShowErrorMessage displays an error in the status bar and clears both tables.
 * Dispatches asynchronously to the main queue.
 */
void ShowErrorMessage(const char *message);

/* ── Go → Cocoa callbacks (implemented in controls.go, called from Obj-C) ── */

/** GoStartMonitoring starts a new monitoring run with the given frame length. */
void GoStartMonitoring(double frameSeconds);

/** GoStopMonitoring cancels the active monitoring run. */
void GoStopMonitoring(void);

/**
 * GoSetHideSmall enables (enabled != 0) or disables filtering of processes
 * that consumed less than 1 CPU-second in the frame.
 */
void GoSetHideSmall(int enabled);

/**
 * GoSetHidePaths enables (enabled != 0) or disables showing only the
 * executable basename instead of the full command line.
 */
void GoSetHidePaths(int enabled);

/**
 * GoSelectFrame switches the UI to the frame at selectedIndex in the history
 * popup. Out-of-range indices are ignored.
 */
void GoSelectFrame(int selectedIndex);

/** GoInitialHideSmall returns the persisted hideSmall setting (1 = on, 0 = off). */
int GoInitialHideSmall(void);

/** GoInitialHidePaths returns the persisted hidePaths setting (1 = on, 0 = off). */
int GoInitialHidePaths(void);

/** GoInitialFrameSeconds returns the persisted frame length in seconds. */
double GoInitialFrameSeconds(void);

#endif
