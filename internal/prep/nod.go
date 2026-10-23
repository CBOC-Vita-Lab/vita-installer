package prep

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// nodtool (https://github.com/encounter/nod, MIT) extracts GameCube and Wii
// disc images. It is downloaded the first time it is needed and checked
// against these hashes.
const nodVersion = "v2.0.0-alpha.12"

var nodAssets = map[string][2]string{
	"darwin/arm64":  {"nodtool-macos-arm64", "524c0b27c0c4289e435df4474d288ea7e63d6a34814cc7c650231688dd1e53b4"},
	"darwin/amd64":  {"nodtool-macos-x86_64", "d1e7ba904c2e557fafbd3bf2d9b75a7901a2bf1568c0fb3a505fca0f30b0a833"},
	"windows/amd64": {"nodtool-windows-x86_64.exe", "f65de62c44f0ad5d179eff85bd1e92099713ffacbf717f651fb761129a25c7bc"},
	"windows/arm64": {"nodtool-windows-arm64.exe", "463d5f7fd7d753fde05c6772f8114ec6f8ab6eaf054790ed9b95a89ef8c63526"},
	"windows/386":   {"nodtool-windows-x86.exe", "293ad6cb4eed336c65e154878106a8edcc920905cb667e5d04a7baa4c3d65621"},
	"linux/amd64":   {"nodtool-linux-x86_64", "ee8345c004248eb82a86dd0c6068e0fcf3dc75baf68d18fe949bf4589822f5a5"},
	"linux/arm64":   {"nodtool-linux-aarch64", "33e801c0e4aee7e714ad84e89521f50e2f79005571b9d4779b695e44ea21ff99"},
	"linux/386":     {"nodtool-linux-i686", "efaa034f4d32a14856d5150c7eee45876ec38c07f75572cb6b4c33423c5850ce"},
}

// ErrNeedsFolder means disc images cannot be converted on this computer; the
// player should extract the disc with Dolphin and choose the folder instead.
var ErrNeedsFolder = errors.New("this computer cannot unpack disc images automatically. In Dolphin, right-click the game > Properties > Filesystem, right-click the disc and choose Extract Entire Disc, then choose that folder here")

// NodURL is the download source; tests replace it.
var NodURL = func(asset string) string {
	return "https://github.com/encounter/nod/releases/download/" + nodVersion + "/" + asset
}

// nodtool returns a verified copy of nodtool in cacheDir, downloading it if needed.
func nodtool(ctx context.Context, cacheDir string) (string, error) {
	asset, ok := nodAssets[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok || tooOldForNod() {
		return "", ErrNeedsFolder
	}
	dir := filepath.Join(cacheDir, "tools")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, asset[0])
	if sum, err := fileSHA256(path); err == nil && sum == asset[1] {
		return path, nil
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", NodURL(asset[0]), nil)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return "", fmt.Errorf("could not download the disc tool (check your internet connection): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("could not download the disc tool (HTTP %d)", resp.StatusCode)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 64<<20))
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if hex.EncodeToString(h.Sum(nil)) != asset[1] {
		os.Remove(tmp)
		return "", errors.New("the downloaded disc tool failed its integrity check")
	}
	os.Chmod(tmp, 0o755)
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ExtractDisc unpacks the game partition of a disc image into out (which must
// not exist) and returns the folder holding sys/ and files/. progress gets
// the bytes written so far and an estimate of the total (0 if unknown).
func ExtractDisc(ctx context.Context, cacheDir, image, out string, progress func(done, total int64)) (string, error) {
	tool, err := nodtool(ctx, cacheDir)
	if err != nil {
		return "", err
	}
	var total int64
	if st, err := os.Stat(image); err == nil && strings.EqualFold(filepath.Ext(image), ".iso") {
		total = st.Size() * 9 / 10
	}
	tmp := out + ".part"
	os.RemoveAll(tmp)
	cmd := exec.CommandContext(ctx, tool, "extract", "-q", image, tmp)
	hideWindow(cmd)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			return "", ErrNeedsFolder
		}
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				os.RemoveAll(tmp)
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				msg := strings.TrimSpace(stderr.String())
				if len(msg) > 300 {
					msg = msg[len(msg)-300:]
				}
				if msg == "" {
					msg = err.Error()
				}
				return "", fmt.Errorf("the disc image could not be unpacked: %s", msg)
			}
			if err := os.Rename(tmp, out); err != nil {
				return "", err
			}
			return FindExtractedRoot(out)
		case <-tick.C:
			if progress != nil {
				progress(dirSize(tmp), total)
			}
		}
	}
}

func dirSize(dir string) int64 {
	var n int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}
