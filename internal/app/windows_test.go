package app

import (
	"slices"
	"testing"
	"time"
)

func TestSerialKeepsOrderAndNeverWaits(t *testing.T) {
	var s serial
	release := make(chan struct{})
	done := make(chan struct{})
	var got []int
	s.do(func() { <-release; got = append(got, 0) })
	for i := 1; i < 5; i++ {
		s.do(func() { got = append(got, i) })
	}
	s.do(func() { close(done) })
	close(release) // reached only if do returned with the first function still running

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the queue did not finish")
	}
	if want := []int{0, 1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Errorf("ran %v, want %v", got, want)
	}
}
