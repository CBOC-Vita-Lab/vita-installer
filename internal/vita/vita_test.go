package vita_test

import (
	"path/filepath"
	"testing"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
	. "github.com/CBOC-Vita-Lab/vita-installer/internal/vita"
)

func TestOpenVPK(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.vpk")
	if err := fakevita.MakeVPK(p, "ABCD12345", "  My   Game ", nil); err != nil {
		t.Fatal(err)
	}
	app, err := OpenVPK(p)
	if err != nil {
		t.Fatal(err)
	}
	if app.TitleID != "ABCD12345" || app.Title != "My Game" || app.Version != "1.00" || app.Icon == nil {
		t.Fatalf("%+v", app)
	}
}

func TestSafeName(t *testing.T) {
	for name, ok := range map[string]bool{"a/b.txt": true, "../x": false, "/abs": false, "c:/x": false, "a/../../b": false, `a\..\b`: false} {
		if SafeName(name) != ok {
			t.Errorf("SafeName(%q) != %v", name, ok)
		}
	}
}
