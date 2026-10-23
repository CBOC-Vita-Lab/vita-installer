package selfupdate_test

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/selfupdate"
)

// TestSelfUpdateE2E builds an old and a new installer, serves the new one as
// a release, and checks both ways of updating: "Update now" in the page, and
// switching by itself at the next start. Run with SELFUPDATE_E2E=1.
func TestSelfUpdateE2E(t *testing.T) {
	if os.Getenv("SELFUPDATE_E2E") == "" {
		t.Skip("set SELFUPDATE_E2E=1 to run")
	}
	root, _ := filepath.Abs("../..")
	tmp := t.TempDir()
	build := func(version, out string) {
		cmd := exec.Command("go", "build", "-o", out, "-ldflags", "-X main.Version="+version, ".")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", version, err, b)
		}
	}
	exeName := "cboc-vita-installer"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	oldBin, newBin := filepath.Join(tmp, "old-"+exeName), filepath.Join(tmp, "new-"+exeName)
	build("1.0.0", oldBin)
	build("9.9.9", newBin)

	// The release: asset + checksums + API answer.
	assetName := "CBOC-Vita-Installer-9.9.9" + selfupdate.AssetSuffix()
	asset := packageAsset(t, newBin, "9.9.9")
	sum := sha256.Sum256(asset)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			base := "http://" + r.Host
			json.NewEncoder(w).Encode(map[string]interface{}{
				"tag_name": "v9.9.9", "html_url": base + "/page",
				"assets": []map[string]string{
					{"name": assetName, "browser_download_url": base + "/" + assetName},
					{"name": "SHA256SUMS.txt", "browser_download_url": base + "/SHA256SUMS.txt"},
				},
			})
		case "/" + assetName:
			w.Write(asset)
		case "/SHA256SUMS.txt":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	for _, mode := range []string{"click", "next-start"} {
		t.Run(mode, func(t *testing.T) {
			dir := filepath.Join(tmp, mode)
			prog := installOld(t, oldBin, dir)
			home := filepath.Join(dir, "home")
			env := append(os.Environ(), "HOME="+home, "XDG_CACHE_HOME="+home+"/cache", "XDG_CONFIG_HOME="+home+"/config",
				"LOCALAPPDATA="+home+"/local", "APPDATA="+home+"/roaming", "CBOC_UPDATE_FEED="+srv.URL+"/release.json")
			addr := freeAddr(t)
			url := start(t, prog, env, addr)
			waitFor(t, 60*time.Second, func() bool {
				s := state(url)
				u, _ := s["update"].(map[string]interface{})
				if u != nil && u["manual"] != nil && u["manual"] != "" {
					t.Fatalf("update not possible: %v", u["manual"])
				}
				return u != nil && u["ready"] == true
			}, "the update to be downloaded")
			if mode == "click" {
				post(t, url, "selfupdate")
			} else {
				post(t, url, "quit")
				waitFor(t, 20*time.Second, func() bool { return state(url) == nil }, "the old version to quit")
				// It switches to the new version while starting; the new
				// one records its page address in running.json.
				cmd := exec.Command(programIn(prog), "-no-browser", "-addr", addr)
				cmd.Env = env
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				go cmd.Wait()
				waitFor(t, 60*time.Second, func() bool {
					url = runningURL(home)
					return url != "" && state(url) != nil
				}, "the installer to start again")
			}
			waitFor(t, 60*time.Second, func() bool {
				s := state(url)
				return s != nil && s["version"] == "9.9.9"
			}, "the new version to take over the page")
			out, err := exec.Command(programIn(prog), "-version").Output()
			if err != nil || strings.TrimSpace(string(out)) != "9.9.9" {
				t.Fatalf("installed program reports %q (%v)", out, err)
			}
			post(t, url, "quit")
			waitFor(t, 20*time.Second, func() bool { return state(url) == nil }, "the new version to quit")
			if _, err := os.Stat(prog + ".old"); err == nil && runtime.GOOS != "windows" {
				t.Errorf("%s.old was left behind", prog)
			}
		})
	}
}

// installOld puts the old version where a player would have it and returns
// the program (or .app) path.
func installOld(t *testing.T, bin, dir string) string {
	os.MkdirAll(dir, 0o755)
	if runtime.GOOS == "darwin" {
		app := filepath.Join(dir, "Applications", "CBOC Vita Installer.app")
		writeApp(t, app, bin, "1.0.0")
		return app
	}
	p := filepath.Join(dir, filepath.Base(bin))
	copyTo(t, bin, p)
	return p
}

func programIn(prog string) string {
	if strings.HasSuffix(prog, ".app") {
		return filepath.Join(prog, "Contents", "MacOS", "CBOC Vita Installer")
	}
	return prog
}

func writeApp(t *testing.T, app, bin, version string) {
	os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755)
	copyTo(t, bin, filepath.Join(app, "Contents", "MacOS", "CBOC Vita Installer"))
	plist, err := os.ReadFile("../../packaging/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), bytes.ReplaceAll(plist, []byte("VERSION"), []byte(version)), 0o644)
	if out, err := exec.Command("codesign", "--force", "--deep", "--sign", "-", app).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v %s", err, out)
	}
}

func packageAsset(t *testing.T, bin, version string) []byte {
	var buf bytes.Buffer
	switch runtime.GOOS {
	case "windows":
		b, _ := os.ReadFile(bin)
		return b
	case "linux":
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		b, _ := os.ReadFile(bin)
		tw.WriteHeader(&tar.Header{Name: "cboc-vita-installer", Mode: 0o755, Size: int64(len(b)), Typeflag: tar.TypeReg})
		tw.Write(b)
		tw.Close()
		gz.Close()
	default:
		app := filepath.Join(t.TempDir(), "CBOC Vita Installer.app")
		writeApp(t, app, bin, version)
		zw := zip.NewWriter(&buf)
		filepath.Walk(app, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(filepath.Dir(app), p)
			h, _ := zip.FileInfoHeader(info)
			h.Name = filepath.ToSlash(rel)
			w, _ := zw.CreateHeader(h)
			b, _ := os.ReadFile(p)
			w.Write(b)
			return nil
		})
		zw.Close()
	}
	return buf.Bytes()
}

func copyTo(t *testing.T, src, dst string) {
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func freeAddr(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// start runs the installer and returns its page address.
func start(t *testing.T, prog string, env []string, addr string) string {
	cmd := exec.Command(programIn(prog), "-no-browser", "-addr", addr)
	cmd.Env = env
	out, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go cmd.Wait()
	urls := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			if i := strings.Index(sc.Text(), "http://"); i >= 0 {
				urls <- sc.Text()[i:]
			}
		}
		io.Copy(io.Discard, out)
	}()
	select {
	case u := <-urls:
		return u
	case <-time.After(30 * time.Second):
		t.Fatal("the installer did not start")
	}
	return ""
}

func state(url string) map[string]interface{} {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(url + "api/state")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var s map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&s) != nil {
		return nil
	}
	return s
}

func post(t *testing.T, url, name string) {
	req, _ := http.NewRequest("POST", url+"api/"+name, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "cboc")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("%s: %s", name, b)
	}
}

func waitFor(t *testing.T, d time.Duration, ok func() bool, what string) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if ok() {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func runningURL(home string) string {
	matches, _ := filepath.Glob(filepath.Join(home, "*", "*", "CBOC Vita Installer", "running.json"))
	more, _ := filepath.Glob(filepath.Join(home, "*", "CBOC Vita Installer", "running.json"))
	matches = append(matches, more...)
	// On a Mac the new version is started by "open", with the real home folder.
	if d, err := os.UserConfigDir(); err == nil {
		matches = append(matches, filepath.Join(d, "CBOC Vita Installer", "running.json"))
	}
	for _, m := range matches {
		var li struct{ URL string }
		if b, err := os.ReadFile(m); err == nil && json.Unmarshal(b, &li) == nil && li.URL != "" {
			return li.URL
		}
	}
	return ""
}
