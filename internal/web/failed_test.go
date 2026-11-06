package web

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
)

func TestFailedStaysFailed(t *testing.T) {
	it := &Item{Status: "failed", Error: "Could not send eboot.bin: 550 no space."}
	setStatus(it) // what the refresh after an install does
	if it.Status != "failed" || it.Error == "" {
		t.Fatalf("a failed install lost its error: %q %q", it.Status, it.Error)
	}
}

func TestFriendlyCardFull(t *testing.T) {
	err := fmt.Errorf("could not send eboot.bin: %w", &ftp.Error{Code: 552, Msg: "No space"})
	if m := friendly(err); !strings.Contains(m, "memory card is full") {
		t.Fatalf("552: %q", m)
	}
	err = fmt.Errorf("could not send eboot.bin: %w", &ftp.Error{Code: 426, Msg: "Connection closed; transfer aborted."})
	if m := friendly(err); !strings.Contains(m, "stopped receiving") {
		t.Fatalf("426: %q", m)
	}
}
