package ftp

import "testing"

func TestParseList(t *testing.T) {
	got := parseList("drwxrwxrwx 1 vita vita 0 Jan  1 00:00 app\r\n-rw-rw-rw- 1 vita vita 1234 Oct 23  2026 level 1.arc\r\n-rw-rw-rw- 1 vita vita 5 Jan 1 00:00 .\r\n")
	if len(got) != 2 || !got[0].IsDir || got[0].Name != "app" || got[1].Name != "level 1.arc" || got[1].Size != 1234 {
		t.Fatalf("%+v", got)
	}
}

func TestPath(t *testing.T) {
	for in, want := range map[string]string{"ux0:data/x": "/ux0:/data/x", "ux0:": "/ux0:/", "ur0:/data": "/ur0:/data"} {
		if got := Path(in); got != want {
			t.Errorf("Path(%q) = %q, want %q", in, got, want)
		}
	}
}
