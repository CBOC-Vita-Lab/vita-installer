package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/selfupdate"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/sys"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/vita"
)

var groupPattern = regexp.MustCompile(`^[a-z0-9]{8,40}$`)

// Server serves the page and its API on 127.0.0.1 behind a random token.
type Server struct {
	App     *App
	Token   string
	Static  fs.FS
	Addr    string
	Quit    chan struct{}
	quit    sync.Once
	clients int
	idle    time.Time
	mu      sync.Mutex
}

// NewServer creates a server for app with the page files in static.
func NewServer(app *App, static fs.FS) *Server {
	b := make([]byte, 16)
	rand.Read(b)
	return &Server{App: app, Token: hex.EncodeToString(b), Static: static, Quit: make(chan struct{}), idle: time.Now()}
}

// Listen binds to addr ("127.0.0.1:0" picks a free port).
func (s *Server) Listen(addr string) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s.Addr = l.Addr().String()
	return l, nil
}

// URL is the page address to open in the browser.
func (s *Server) URL() string { return "http://" + s.Addr + "/" + s.Token + "/" }

// Serve handles requests until the listener closes.
func (s *Server) Serve(l net.Listener) error {
	return (&http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}).Serve(l)
}

func (s *Server) stop() {
	time.Sleep(200 * time.Millisecond)
	s.quit.Do(func() { close(s.Quit) })
}

// IdleFor reports how long no page has been open (0 while one is).
func (s *Server) IdleFor() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients > 0 {
		return 0
	}
	return time.Since(s.idle)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only this computer, only our own page: refuse other hosts (DNS rebinding) and other sites.
	if r.Host != s.Addr && r.Host != strings.Replace(s.Addr, "127.0.0.1", "localhost", 1) {
		http.Error(w, "forbidden", 403)
		return
	}
	prefix := "/" + s.Token + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, "not found", 404)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, prefix)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	if strings.HasPrefix(p, "api/") {
		if r.Method == "POST" && r.Header.Get("X-Requested-With") != "cboc" {
			http.Error(w, "forbidden", 403)
			return
		}
		s.api(w, r, strings.TrimPrefix(p, "api/"))
		return
	}
	if p == "" {
		p = "index.html"
	}
	b, err := fs.ReadFile(s.Static, p)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	switch filepath.Ext(p) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	w.Write(b)
}

func reply(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(400)
		msg := err.Error()
		if msg != "" {
			msg = strings.ToUpper(msg[:1]) + msg[1:]
		}
		json.NewEncoder(w).Encode(map[string]string{"error": msg})
		return
	}
	w.Write([]byte(`{"ok":true}`))
}

func (s *Server) api(w http.ResponseWriter, r *http.Request, name string) {
	a := s.App
	var body struct {
		Path, Host, Drive, URL, Item, Input, Kind string
	}
	if r.Method == "POST" && name != "upload" {
		json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	}
	switch name {
	case "ping":
		w.Write([]byte("ok"))
	case "state":
		w.Header().Set("Content-Type", "application/json")
		w.Write(a.Snapshot())
	case "events":
		s.events(w, r)
	case "icon":
		a.mu.Lock()
		var icon []byte
		for _, it := range a.st.Items {
			if it.ID == r.URL.Query().Get("id") && it.App != nil {
				icon = it.App.Icon
			}
		}
		a.mu.Unlock()
		if icon == nil {
			http.Error(w, "no icon", 404)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Write(icon)
	case "scan":
		go a.Scan(context.Background())
		reply(w, nil)
	case "connect":
		reply(w, a.Connect(r.Context(), body.Host, body.Drive))
	case "add":
		reply(w, a.Add(body.Path))
	case "url":
		reply(w, a.AddURL(context.Background(), body.URL))
	case "pick":
		// Native file dialog; the chosen path goes to a release or an input.
		prompt := "Choose a Vita app (.vpk) or release ZIP"
		if body.Kind == "folder" {
			prompt = "Choose a release folder"
		}
		if body.Input != "" {
			prompt = "Choose the file for this game"
		}
		kind := body.Kind
		if kind == "" {
			kind = "file"
		}
		p, ok := sys.Pick(kind, prompt)
		if !ok {
			a.update(func(st *State) { st.Dialogs = false })
			reply(w, fmt.Errorf("file dialogs are not available here. Drag the file onto this window instead"))
			return
		}
		if p == "" {
			reply(w, nil)
			return
		}
		if body.Input != "" {
			reply(w, a.SetInput(body.Item, body.Input, p))
		} else if body.Kind == "drive" {
			reply(w, a.Connect(r.Context(), "", p))
		} else {
			reply(w, a.Add(p))
		}
	case "upload":
		s.upload(w, r)
	case "uploaded":
		// A dragged folder has arrived; use it like a chosen folder.
		var b struct{ Group, Root, Item, Input string }
		json.Unmarshal([]byte(body.Path), &b)
		if !groupPattern.MatchString(b.Group) || !vita.SafeName(b.Root) {
			reply(w, fmt.Errorf("unsafe folder"))
			return
		}
		p := filepath.Join(a.cache, "uploads", b.Group, filepath.FromSlash(b.Root))
		if b.Input != "" {
			reply(w, a.SetInput(b.Item, b.Input, p))
		} else {
			reply(w, a.Add(p))
		}
	case "input":
		reply(w, a.SetInput(body.Item, body.Input, body.Path))
	case "remove":
		a.Remove(body.Item)
		reply(w, nil)
	case "install":
		reply(w, a.Install(body.Item))
	case "cancel":
		a.Cancel()
		reply(w, nil)
	case "launch":
		reply(w, a.Launch(body.Item))
	case "dismiss":
		a.update(func(st *State) { st.Notice = "" })
		reply(w, nil)
	case "open":
		if strings.HasPrefix(body.URL, "https://github.com/") {
			sys.OpenURL(body.URL)
		}
		reply(w, nil)
	case "quit":
		reply(w, nil)
		go s.stop()
	case "selfupdate":
		st := a.Staged()
		switch {
		case st == nil:
			reply(w, errors.New("the update is still downloading"))
		case a.Busy():
			reply(w, errors.New("wait until the installation finishes, then update"))
		default:
			// The new version takes over this page's address, so the open
			// page reconnects to it by itself.
			if err := selfupdate.Apply(st, []string{"-addr", s.Addr, "-token", s.Token, "-no-browser"}); err != nil {
				selfupdate.MarkFailed(a.cache, st.Version)
				a.mu.Lock()
				a.staged = nil
				a.mu.Unlock()
				a.update(func(x *State) {
					if x.Update != nil {
						x.Update.Ready, x.Update.Manual = false, "it could not replace itself on this computer"
					}
				})
				reply(w, fmt.Errorf("the update could not be installed (%v). Download it instead", err))
				return
			}
			reply(w, nil)
			go s.stop()
		}
	default:
		http.Error(w, "not found", 404)
	}
}

// upload receives a dragged file. It is saved once in the cache, then used
// like a chosen file.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := filepath.Base(filepath.Clean("/" + q.Get("name")))
	if name == "" || name == "/" || name == "." {
		reply(w, fmt.Errorf("missing file name"))
		return
	}
	dir := filepath.Join(s.App.cache, "uploads")
	dest := filepath.Join(dir, name)
	// Part of a dragged folder: kept under its group with its relative path.
	if group, rel := q.Get("group"), q.Get("rel"); group != "" {
		if !groupPattern.MatchString(group) || !vita.SafeName(rel) {
			reply(w, fmt.Errorf("unsafe file path"))
			return
		}
		dest = filepath.Join(dir, group, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(dest), 0o755)
		f, err := os.Create(dest)
		if err == nil {
			_, err = io.Copy(f, r.Body)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			reply(w, fmt.Errorf("could not read %s: %v", name, err))
			return
		}
		if ms := q.Get("modified"); ms != "" {
			var v int64
			fmt.Sscan(ms, &v)
			t := time.Unix(0, v*1e6)
			os.Chtimes(dest, t, t)
		}
		reply(w, nil)
		return
	}
	os.MkdirAll(dir, 0o755)
	size := r.ContentLength
	if st, err := os.Stat(dest); err == nil && size > 0 && st.Size() == size && q.Get("modified") != "" &&
		fmt.Sprint(st.ModTime().UnixNano()/1e6) == q.Get("modified") {
		io.Copy(io.Discard, r.Body) // same file as last time
	} else {
		tmp := dest + ".part"
		f, err := os.Create(tmp)
		if err != nil {
			reply(w, err)
			return
		}
		s.App.update(func(st *State) { st.Task = &Task{Text: "Reading " + name, Total: size} })
		buf := make([]byte, 1<<20)
		var done int64
		last := time.Now()
		var werr error
		for {
			n, rerr := r.Body.Read(buf)
			if n > 0 {
				if _, err := f.Write(buf[:n]); err != nil {
					werr = err
					break
				}
				done += int64(n)
				if time.Since(last) > 200*time.Millisecond {
					last = time.Now()
					d := done
					s.App.update(func(st *State) {
						if st.Task != nil {
							st.Task.Done = d
						}
					})
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				werr = rerr
				break
			}
		}
		f.Close()
		s.App.update(func(st *State) { st.Task = nil })
		if werr != nil {
			os.Remove(tmp)
			reply(w, fmt.Errorf("could not read the file: %v", werr))
			return
		}
		os.Remove(dest)
		os.Rename(tmp, dest)
		if ms := q.Get("modified"); ms != "" {
			var v int64
			fmt.Sscan(ms, &v)
			t := time.Unix(0, v*1e6)
			os.Chtimes(dest, t, t)
		}
	}
	if item := q.Get("item"); item != "" {
		reply(w, s.App.SetInput(item, q.Get("input"), dest))
		return
	}
	reply(w, s.App.Add(dest))
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	s.mu.Lock()
	s.clients++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.clients--
		s.idle = time.Now()
		s.mu.Unlock()
	}()
	c, done := s.App.Subscribe()
	defer done()
	send := func() bool {
		_, err := fmt.Fprintf(w, "data: %s\n\n", s.App.Snapshot())
		fl.Flush()
		return err == nil
	}
	if !send() {
		return
	}
	keep := time.NewTicker(15 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c:
			time.Sleep(60 * time.Millisecond) // coalesce bursts
			select {
			case <-c:
			default:
			}
			if !send() {
				return
			}
		case <-keep.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
