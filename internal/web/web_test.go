package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestServerGuards(t *testing.T) {
	app := NewApp("1.0.0", t.TempDir(), t.TempDir())
	s := NewServer(app, fstest.MapFS{"index.html": {Data: []byte("<html>")}})
	s.Addr = "127.0.0.1:9999"
	do := func(method, path, host string, hdr bool) int {
		r := httptest.NewRequest(method, path, strings.NewReader("{}"))
		r.Host = host
		if hdr {
			r.Header.Set("X-Requested-With", "cboc")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w.Code
	}
	tok := "/" + s.Token + "/"
	cases := []struct {
		method, path, host string
		hdr                bool
		want               int
	}{
		{"GET", tok, s.Addr, false, 200},
		{"GET", "/", s.Addr, false, 404},
		{"GET", tok, "evil.example:9999", false, 403},
		{"POST", tok + "api/dismiss", s.Addr, false, 403},
		{"POST", tok + "api/dismiss", s.Addr, true, 200},
		{"GET", tok + "api/state", s.Addr, false, 200},
	}
	for _, c := range cases {
		if got := do(c.method, c.path, c.host, c.hdr); got != c.want {
			t.Errorf("%s %s host=%s hdr=%v: %d, want %d", c.method, c.path, c.host, c.hdr, got, c.want)
		}
	}
	_ = http.StatusOK
}

func TestNewer(t *testing.T) {
	if !newer("v1.2.0", "1.1.9") || newer("v1.0.0", "1.0.0") || newer("v0.1.11", "1.0.0") || !newer("v1.0.1", "1.0.0") {
		t.Fatal("version compare")
	}
}

func TestRepoPattern(t *testing.T) {
	for _, u := range []string{"https://github.com/CBOC-Vita-Lab/strikers-charged-vita", "github.com/a/b.git", "https://github.com/a/b/releases/tag/v1"} {
		if repoPattern.FindStringSubmatch(u) == nil {
			t.Errorf("%s not matched", u)
		}
	}
}
