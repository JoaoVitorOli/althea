//go:build linux

package linux

import "github.com/egoist/mygo"

// SingleInstance uses MyGo's lock, a Unix socket, which Linux paths of any name support.
type SingleInstance struct{}

// Lock implements platform.SingleInstance.
func (SingleInstance) Lock(_ string, onSecond func()) (first bool, release func(), err error) {
	if !mygo.App.RequestSingleInstanceLock() {
		return false, func() {}, nil
	}
	off := mygo.App.OnSecondInstance(func([]string, string) { onSecond() })
	return true, off, nil
}
