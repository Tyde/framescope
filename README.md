# FrameScope

A macOS CPU profiler that works by sampling process CPU time across fixed-length frames, making it easy to see which processes consumed the most CPU over a given window rather than just a point-in-time snapshot.

![FrameScope icon](FrameScopeIcon.png)

## What it does

Most system monitors show instantaneous CPU usage. FrameScope instead measures how many **CPU-seconds** each process consumed over a configurable time window (a "frame"). This makes it straightforward to answer questions like "what chewed through the most CPU in the last 30 seconds?" without watching a live graph.

- Define a frame length (e.g. 15 seconds)
- FrameScope records per-process CPU time at the start and end of each frame
- Results are sorted by CPU consumption for that frame
- Completed frames are kept in a history you can navigate back through
- A summary table shows totals and per-frame averages across all recorded frames

## Requirements

- macOS (uses AppKit/Cocoa for the UI)
- Go 1.21+
- Xcode command-line tools (`xcode-select --install`)

## Building

```sh
git clone https://github.com/Tyde/framescope.git
cd framescope
go build -o FrameScope .
```

Run the resulting binary directly:

```sh
./FrameScope
```

To build a proper `.app` bundle, wrap it using standard macOS bundle structure or your preferred packaging tool.

## Usage

1. Set the **frame length** in the toolbar (default: 15 seconds).
2. Click **Start** to begin monitoring.
3. When a frame completes it moves to the history list; click **‹ Prev** / **Next ›** or use the dropdown to browse frames.
4. Click **Stop** at any time — the last completed frame stays selected.

### Toolbar options

| Control | Description |
|---|---|
| Frame (s) | Duration of each measurement window in seconds |
| Start / Stop | Begin or end monitoring |
| Hide <1s | Filter out processes that used less than 1 CPU-second in the frame |
| Basename only | Show only the executable name, not the full command line path |

### Reading the tables

**Current Frame table** — rows for the active or selected frame:

| Column | Meaning |
|---|---|
| PID | Process ID |
| CPU-s | CPU-seconds consumed in the frame |
| Duration | Same value formatted as HH:MM:SS |
| Command | Process name or command line |

**Summary table** — aggregated across all recorded frames:

| Column | Meaning |
|---|---|
| PID | Process ID |
| Total CPU-s | Total CPU-seconds across all frames |
| Avg CPU-s | Average per frame |
| Total / Avg Duration | Same values as HH:MM:SS |
| Command | Process name or command line |

## Architecture

FrameScope is a Go application that embeds a native macOS UI via cgo.

```
main.go            — entry point; locks OS thread, calls RunApp()
monitor.go         — sampling loop; diffs CPU times across a frame
compute.go         — per-process CPU diff calculation and sorting
render.go          — formats result rows as tab-separated text for the UI
state.go           — shared monitorState struct (mutex-protected)
model.go           — data types (processSample, resultRow, frameRecord, …)
controls.go        — exported Go functions called from Cocoa (GoStart, GoStop, …)
ui_bridge.go       — Go→Cocoa calls (pushUI, postUpdate, postError)
config.go          — load/save settings (~/Library/Application Support/FrameScope/)
cocoa_bridge.h/.m  — AppKit UI: NSToolbar, NSSplitView, NSTableView, status bar
```

Process information is collected via [gopsutil](https://github.com/shirou/gopsutil).

## Configuration

Settings are saved automatically to:

```
~/Library/Application Support/FrameScope/config.json
```

The file stores the last-used frame length and the Hide/Basename checkbox states. It is created on first save and ignored if absent or malformed.

## License

MIT

---

## Notice

This project was created entirely with the assistance of large language models — the initial codebase was written with [OpenAI Codex](https://openai.com/index/openai-codex/) and subsequently developed and maintained with [Claude](https://claude.ai) (Anthropic). No code was written by hand.
