//go:build windows

package windows

import (
	"fmt"
	"testing"
	"time"
)

func TestSingleInstance(t *testing.T) {
	name := fmt.Sprintf("althea-test-%d", time.Now().UnixNano())
	shown := make(chan struct{}, 1)

	first, release, err := SingleInstance{}.Lock(name, func() { shown <- struct{}{} })
	if err != nil || !first {
		t.Fatalf("first Lock = %v, %v", first, err)
	}
	defer release()

	// A second launch (here, a second lock of the same name) signals the first and backs off.
	second, release2, err := SingleInstance{}.Lock(name, func() { t.Error("the second instance was signaled") })
	if err != nil || second {
		t.Fatalf("second Lock = %v, %v; want false", second, err)
	}
	release2()
	select {
	case <-shown:
	case <-time.After(2 * time.Second):
		t.Fatal("the first instance was not told about the second")
	}
}
