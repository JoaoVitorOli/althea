//go:build windows

package windows

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

var procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")

const asfwAny = ^uintptr(0) // ASFW_ANY

// SingleInstance keeps Althea to one instance per user with a named mutex, and tells the first
// instance of a second launch through a named event. Unlike a socket in the temp folder, it
// works whatever the user's folder is called: AF_UNIX sockets cannot connect through paths with
// accents, as C:\Users\João.
type SingleInstance struct{}

// Lock implements platform.SingleInstance.
func (SingleInstance) Lock(name string, onSecond func()) (first bool, release func(), err error) {
	mutexName, err := windows.UTF16PtrFromString(`Local\` + name + `.lock`)
	if err != nil {
		return false, nil, err
	}
	eventName, err := windows.UTF16PtrFromString(`Local\` + name + `.show`)
	if err != nil {
		return false, nil, err
	}

	mutex, err := windows.CreateMutex(nil, false, mutexName)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		// Another instance runs: let it come to the front, wake it, and back off.
		_ = windows.CloseHandle(mutex)
		procAllowSetForegroundWindow.Call(asfwAny)
		event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, eventName)
		if err != nil {
			return false, func() {}, fmt.Errorf("single instance: %w", err)
		}
		defer func() { _ = windows.CloseHandle(event) }()
		return false, func() {}, windows.SetEvent(event)
	}
	if err != nil {
		return false, nil, fmt.Errorf("single instance: %w", err)
	}

	event, err := windows.CreateEvent(nil, 0, 0, eventName) // auto-reset, not signaled
	if err != nil {
		_ = windows.CloseHandle(mutex)
		return false, nil, fmt.Errorf("single instance: %w", err)
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(event)
		_ = windows.CloseHandle(mutex)
		return false, nil, fmt.Errorf("single instance: %w", err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			r, err := windows.WaitForMultipleObjects([]windows.Handle{event, stop}, false, windows.INFINITE)
			if err != nil || r != windows.WAIT_OBJECT_0 {
				return // stopped
			}
			onSecond()
		}
	})
	var once sync.Once
	return true, func() {
		once.Do(func() {
			_ = windows.SetEvent(stop)
			wg.Wait()
			for _, h := range []windows.Handle{stop, event, mutex} {
				_ = windows.CloseHandle(h)
			}
		})
	}, nil
}
