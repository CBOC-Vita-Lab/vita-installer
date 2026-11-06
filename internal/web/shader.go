package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/install"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/release"
)

// ShaRKBR33D is a free Vita app by Rinnegatamante that gets the shader
// compiler (libshacccg.suprx) onto the Vita legally: it downloads Sony's PSM
// runtime on the Vita and takes the file out of it. The installer can put the
// app on the Vita for the player. It never carries the compiler itself.
const sharkRepo = "Rinnegatamante/ShaRKBR33D"

// sharkLatestURL is where the newest ShaRKBR33D release is described. Tests
// replace it.
var sharkLatestURL = "https://api.github.com/repos/" + sharkRepo + "/releases/latest"

// sharkKnown lists apps whose SHA-256 is trusted when GitHub gave no digest
// (releases uploaded before GitHub started adding one have none). The key is
// the checksum, the value the release it belongs to.
var sharkKnown = map[string]string{
	"283856eaad4f99c72ee3d6b0db16ce76dc4716f1d6ef2e9fccf9e65a4acba9af": "v.1.2",
}

// ShaderState is what the page knows about the shader compiler on the Vita.
type ShaderState struct {
	install.ShaderStatus
	Message string `json:"message,omitempty"` // what to do next, in plain words
}

const sharkOpenIt = "On your Vita, open ShaRKBR33D once and follow it (it needs Wi-Fi); then come back and press Check again."

// refreshShader looks at the Vita and updates what the page shows.
func (a *App) refreshShader(ctx context.Context) {
	a.mu.Lock()
	v, drive := a.st.Vita, a.st.Drive
	a.mu.Unlock()
	st := install.CheckShader(ctx, v, drive)
	a.update(func(s *State) {
		msg := ""
		switch {
		case st.Present:
		case st.Helper && st.Known:
			msg = sharkOpenIt
		case st.Helper && s.Shader != nil: // a drive: keep what the install told the player
			msg = s.Shader.Message
		}
		s.Shader = &ShaderState{ShaderStatus: st, Message: msg}
	})
}

// CheckShader is the page's "Check again": it looks at the Vita again, and
// rows that were waiting for the compiler are updated.
func (a *App) CheckShader(ctx context.Context) error {
	a.mu.Lock()
	v, drive := a.st.Vita, a.st.Drive
	a.mu.Unlock()
	if v == nil && drive == "" {
		return errors.New("connect your Vita first")
	}
	if drive != "" {
		return errors.New("the shader compiler can't be checked through a drive. Use Wi-Fi, or choose the file yourself")
	}
	a.refreshShader(ctx)
	a.refreshItems()
	a.mu.Lock()
	st := a.st.Shader
	a.mu.Unlock()
	switch {
	case st == nil || !st.Known:
		return errors.New("could not look at your Vita. Keep VitaShell open and try again")
	case st.Present:
		a.update(func(s *State) { s.Shader.Message = "" })
		a.notice("Your Vita has the shader compiler now.", false)
	case st.Helper:
		a.update(func(s *State) { s.Shader.Message = sharkOpenIt })
		a.notice("Not there yet. "+sharkOpenIt, false)
	default:
		a.notice("Your Vita doesn't have the shader compiler yet.", true)
	}
	return nil
}

// PrepareShader puts ShaRKBR33D on the Vita like any other app. It runs in
// the background with the same progress sheet as an install.
func (a *App) PrepareShader() error {
	a.mu.Lock()
	if a.st.Job != nil || a.st.Task != nil {
		a.mu.Unlock()
		return errors.New("wait until the current job is finished")
	}
	v, drive := a.st.Vita, a.st.Drive
	a.mu.Unlock()
	if v == nil && drive == "" {
		return errors.New("connect your Vita first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelJob = cancel
	a.update(func(s *State) {
		s.Job = &Job{Name: "ShaRKBR33D", Count: 1, Progress: install.Progress{Phase: "prepare", Text: "Looking up ShaRKBR33D", ETA: -1}}
		s.Notice = ""
	})
	go func() {
		defer cancel()
		res, err := a.installShark(ctx, v, drive)
		a.update(func(s *State) { s.Job = nil })
		if err != nil {
			if ctx.Err() != nil {
				a.notice("Stopped.", true)
			} else {
				a.notice(friendly(err), true)
			}
			return
		}
		msg := sharkOpenIt
		if !res.Installed && len(res.Steps) > 0 {
			msg = strings.Join(res.Steps, " ") + " Then " + strings.ToLower(msg[:1]) + msg[1:]
		}
		a.update(func(s *State) {
			st := install.ShaderStatus{Known: v != nil, Helper: true}
			if s.Shader != nil {
				st.Present = s.Shader.Present
			}
			s.Shader = &ShaderState{ShaderStatus: st, Message: msg}
		})
		a.notice("ShaRKBR33D is on your Vita. "+msg, false)
		a.refreshItems()
	}()
	return nil
}

// installShark downloads the newest ShaRKBR33D release, checks it and installs
// it on the Vita.
func (a *App) installShark(ctx context.Context, v *device.Vita, drive string) (*install.Result, error) {
	var rel ghRelease
	if err := getJSON(ctx, sharkLatestURL, &rel); err != nil {
		return nil, errors.New("could not find ShaRKBR33D on GitHub. Check your internet connection and try again")
	}
	var pick *asset
	for i, as := range rel.Assets {
		if strings.EqualFold(filepath.Ext(as.Name), ".vpk") {
			if pick == nil || strings.EqualFold(as.Name, "ShaRKBR33D.vpk") {
				pick = &rel.Assets[i]
			}
		}
	}
	if pick == nil {
		return nil, errors.New("the latest ShaRKBR33D release has no app to download")
	}
	dir := filepath.Join(a.cache, "downloads", "sharkbr33d-"+safeTag(rel.Tag))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(dir, "ShaRKBR33D.vpk")
	os.Remove(dest) // always verify a fresh copy
	if err := a.downloadOnce(ctx, pick.URL, dest, "ShaRKBR33D"); err != nil {
		return nil, err
	}
	if err := verifyShark(dest, pick.Digest); err != nil {
		os.Remove(dest)
		return nil, err
	}
	rs, err := release.Load(dest, filepath.Join(a.cache, "work"))
	if err != nil || len(rs) != 1 || rs[0].TitleID != install.SharkTitleID {
		os.Remove(dest)
		return nil, errors.New("the file downloaded as ShaRKBR33D is not the ShaRKBR33D app")
	}
	r := rs[0]
	r.Inputs = nil // the app itself needs nothing from the player
	r.Repo, r.Tag = sharkRepo, rel.Tag
	return install.Run(ctx, r, install.Options{
		Vita: v, Drive: drive, Cache: filepath.Join(a.cache, "work"),
		OnProgress: func(p install.Progress) {
			a.update(func(s *State) {
				if s.Job != nil {
					s.Job.Progress = p
				}
			})
		},
	})
}

// verifyShark checks the download against GitHub's digest ("sha256:…"). A
// release GitHub gave no digest for is accepted only if it is one this
// installer knows.
func verifyShark(path, digest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	want := strings.TrimPrefix(strings.ToLower(digest), "sha256:")
	if len(want) == 64 {
		if got != want {
			return errors.New("ShaRKBR33D was damaged on the way (checksum mismatch). Try again")
		}
		return nil
	}
	if _, ok := sharkKnown[got]; ok {
		return nil
	}
	return fmt.Errorf("ShaRKBR33D could not be verified: GitHub gave no checksum for this release. Choose the shader compiler file yourself instead")
}

func safeTag(tag string) string {
	var b strings.Builder
	for _, r := range tag {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "latest"
	}
	return b.String()
}
