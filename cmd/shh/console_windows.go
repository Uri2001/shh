//go:build windows

package main

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	consoleState     consoleModeState
	consoleStateOnce sync.Once
)

type consoleModeState struct {
	inputMode  uint32
	outputMode uint32
	captured   bool
}

func captureConsoleState() {
	consoleStateOnce.Do(func() {
		var state consoleModeState
		if in, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE); err == nil {
			_ = windows.GetConsoleMode(in, &state.inputMode)
		}
		if out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil {
			_ = windows.GetConsoleMode(out, &state.outputMode)
		}
		state.captured = true
		consoleState = state
	})
}

func restoreConsoleState() {
	if !consoleState.captured {
		return
	}
	if in, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE); err == nil {
		_ = windows.SetConsoleMode(in, consoleState.inputMode)
	}
	if out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil {
		_ = windows.SetConsoleMode(out, consoleState.outputMode)
	}
}
