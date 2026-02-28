package main

import (
	"encoding/json"
	"os"
)

const configFilePath = "framescope_config.json"

type appConfig struct {
	HideSmall    bool    `json:"hide_small"`
	HidePaths    bool    `json:"hide_paths"`
	FrameSeconds float64 `json:"frame_seconds"`
}

func initializeConfig() {
	data, err := os.ReadFile(configFilePath)
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

	_ = os.WriteFile(configFilePath, data, 0644)
}
