package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// configPath returns the absolute path to the application's JSON config file:
//
//	~/Library/Application Support/FrameScope/config.json
//
// The directory is not guaranteed to exist; callers that write must create it.
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "FrameScope", "config.json"), nil
}

// appConfig is the serialised form of user preferences persisted to disk.
type appConfig struct {
	HideSmall    bool    `json:"hide_small"`
	HidePaths    bool    `json:"hide_paths"`
	FrameSeconds float64 `json:"frame_seconds"`
}

// initializeConfig loads persisted settings from disk and applies them to the
// global state before the UI starts. Errors are silently ignored — missing or
// malformed config files are treated as "use defaults".
func initializeConfig() {
	path, err := configPath()
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var cfg appConfig
	if json.Unmarshal(data, &cfg) != nil {
		return
	}

	state.mu.Lock()
	state.hideSmall = cfg.HideSmall
	state.hidePaths = cfg.HidePaths
	if cfg.FrameSeconds > 0 {
		state.frameSeconds = cfg.FrameSeconds
	}
	state.mu.Unlock()
}

// saveConfig writes the current user preferences to disk as JSON. The config
// directory is created if it does not already exist. Write errors are silently
// ignored — a failed save does not affect the running session.
func saveConfig() {
	state.mu.Lock()
	cfg := appConfig{
		HideSmall:    state.hideSmall,
		HidePaths:    state.hidePaths,
		FrameSeconds: state.frameSeconds,
	}
	state.mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}

	path, err := configPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
