//go:build windows

package main

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	consoleState   consoleModeState
	consoleStateMu sync.Mutex
)

type consoleModeState struct {
	inputMode  uint32
	outputMode uint32
	haveInput  bool
	haveOutput bool
	captured   bool
}

func captureConsoleState() {
	consoleStateMu.Lock()
	defer consoleStateMu.Unlock()

	if consoleState.captured {
		return
	}

	var state consoleModeState

	if in, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE); err == nil && in != windows.InvalidHandle {
		if err := windows.GetConsoleMode(in, &state.inputMode); err == nil {
			state.haveInput = true
		}
	}
	if out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil && out != windows.InvalidHandle {
		if err := windows.GetConsoleMode(out, &state.outputMode); err == nil {
			state.haveOutput = true
		}
	}

	if state.haveInput || state.haveOutput {
		state.captured = true
		consoleState = state
	}
}

func restoreConsoleState() {
	consoleStateMu.Lock()
	defer consoleStateMu.Unlock()

	if !consoleState.captured {
		return
	}

	if consoleState.haveInput {
		if in, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE); err == nil && in != windows.InvalidHandle {
			_ = windows.SetConsoleMode(in, consoleState.inputMode)
		}
	}

	if consoleState.haveOutput {
		if out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil && out != windows.InvalidHandle {
			_ = windows.SetConsoleMode(out, consoleState.outputMode)
		}
	}

	consoleState = consoleModeState{}
}
