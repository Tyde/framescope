package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "FrameScope", "config.json"), nil
}

type appConfig struct {
	HideSmall    bool    `json:"hide_small"`
	HidePaths    bool    `json:"hide_paths"`
	FrameSeconds float64 `json:"frame_seconds"`
}

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
