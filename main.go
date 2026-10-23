// CBOC Vita Installer: install any PS Vita app or game from a computer.
//
// It runs a small local web page (only reachable from this computer) and
// talks to the Vita over Wi-Fi (VitaShell FTP or Vita Companion) or through
// a mounted memory card / USB drive.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/device"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/sys"
	"github.com/CBOC-Vita-Lab/vita-installer/internal/web"
)

// Version is set at build time.
var Version = "1.0.0"

//go:embed ui
var uiFiles embed.FS

const appName = "CBOC Vita Installer"

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "address for the local page")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser")
	version := flag.Bool("version", false, "print the version")
	companionPort := flag.Int("companion-port", device.CompanionPort, "Vita Companion command port (for testing)")
	flag.Parse()
	device.CompanionPort = *companionPort
	if *version {
		fmt.Println(Version)
		return
	}
	sys.Init()
	if err := run(*addr, !*noBrowser); err != nil {
		sys.Alert(appName, err.Error())
		os.Exit(1)
	}
}

func dirs() (cache, config string) {
	c, err := os.UserCacheDir()
	if err != nil {
		c = os.TempDir()
	}
	g, err := os.UserConfigDir()
	if err != nil {
		g = c
	}
	return filepath.Join(c, appName), filepath.Join(g, appName)
}

type lockInfo struct{ URL string }

// alreadyRunning opens the existing window instead of starting twice.
func alreadyRunning(config string, openBrowser bool) bool {
	b, err := os.ReadFile(filepath.Join(config, "running.json"))
	if err != nil {
		return false
	}
	var li lockInfo
	if json.Unmarshal(b, &li) != nil || li.URL == "" {
		return false
	}
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get(li.URL + "api/ping")
	if err != nil {
		return false
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	if openBrowser {
		sys.OpenURL(li.URL)
	}
	return true
}

func run(addr string, openBrowser bool) error {
	cache, config := dirs()
	os.MkdirAll(config, 0o755)
	if addr == "127.0.0.1:0" && alreadyRunning(config, openBrowser) {
		return nil
	}
	static, _ := fs.Sub(uiFiles, "ui")
	app := web.NewApp(Version, cache, config)
	srv := web.NewServer(app, static)
	l, err := srv.Listen(addr)
	if err != nil {
		return fmt.Errorf("could not start: %v", err)
	}
	lock := filepath.Join(config, "running.json")
	b, _ := json.Marshal(lockInfo{URL: srv.URL()})
	os.WriteFile(lock, b, 0o600)
	defer os.Remove(lock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go app.Watch(ctx)
	go app.CheckOnline(ctx)
	go srv.Serve(l)
	fmt.Println(appName, Version, "is running at", srv.URL())
	if openBrowser {
		if err := sys.OpenURL(srv.URL()); err != nil {
			sys.Alert(appName, "Open this address in your web browser:\n"+srv.URL())
		}
	}
	// Quit when asked, or a while after the last browser tab closes.
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-srv.Quit:
			return nil
		case <-tick.C:
			if srv.IdleFor() > 90*time.Second && !app.Busy() {
				return nil
			}
		}
	}
}
