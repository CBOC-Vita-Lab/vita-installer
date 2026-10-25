package web

import "testing"

func TestFailedStaysFailed(t *testing.T) {
	it := &Item{Status: "failed", Error: "Could not send eboot.bin: 550 no space."}
	setStatus(it) // what the refresh after an install does
	if it.Status != "failed" || it.Error == "" {
		t.Fatalf("a failed install lost its error: %q %q", it.Status, it.Error)
	}
}
