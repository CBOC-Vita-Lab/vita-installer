// Package device finds a PS Vita on the local network or as a mounted drive.
package device

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
)

// FTPPort is the standard Vita FTP port.
const FTPPort = 1337

// CompanionPort is Vita Companion's command port (a variable for tests).
var CompanionPort = 1338

// Vita is a console reachable over Wi-Fi.
type Vita struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Companion bool   `json:"companion"` // Vita Companion answers on 1338: apps can be installed and launched remotely
	HasUR0    bool   `json:"ur0"`
}

// Addr is host:port of the FTP service.
func (v Vita) Addr() string { return net.JoinHostPort(v.Host, fmt.Sprint(v.Port)) }

// Probe checks one address for a Vita FTP service.
func Probe(ctx context.Context, host string, port int) (*Vita, error) {
	if port == 0 {
		port = FTPPort
	}
	c, err := ftp.Dial(ctx, net.JoinHostPort(host, fmt.Sprint(port)), 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if err := c.Cwd("ux0:"); err != nil {
		return nil, fmt.Errorf("%s has an FTP server, but it is not a Vita", host)
	}
	v := &Vita{Host: host, Port: port}
	v.HasUR0 = c.Cwd("ur0:") == nil
	if out, err := Companion(ctx, host, "version", 1500*time.Millisecond); err == nil && strings.Contains(strings.ToLower(out), "companion") {
		v.Companion = true
	}
	return v, nil
}

// Companion sends one command to Vita Companion and returns its reply.
func Companion(ctx context.Context, host, command string, wait time.Duration) (string, error) {
	d := net.Dialer{Timeout: wait}
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(CompanionPort)))
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(wait))
	if _, err := io.WriteString(c, command+"\n"); err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(bufio.NewReader(c), 8192))
	if len(b) > 0 {
		return strings.TrimSpace(string(b)), nil
	}
	return "", err
}

// Scan looks for Vitas on every local IPv4 network (up to 256 addresses each),
// trying preferred hosts first. It returns as soon as the scan finishes.
func Scan(ctx context.Context, preferred []string) []Vita {
	hosts := candidateHosts(preferred)
	var (
		mu    sync.Mutex
		found []Vita
		seen  = map[string]bool{}
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 96)
	)
	for _, h := range hosts {
		h := h
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			d := net.Dialer{Timeout: 700 * time.Millisecond}
			c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(h, fmt.Sprint(FTPPort)))
			if err != nil {
				return
			}
			c.Close()
			v, err := Probe(ctx, h, FTPPort)
			if err != nil {
				return
			}
			mu.Lock()
			if !seen[h] {
				seen[h] = true
				found = append(found, *v)
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Slice(found, func(i, j int) bool { return found[i].Host < found[j].Host })
	return found
}

func candidateHosts(preferred []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(h string) {
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, h := range preferred {
		add(h)
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		if strings.HasPrefix(name, "utun") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "vmnet") ||
			strings.HasPrefix(name, "vboxnet") || strings.HasPrefix(name, "tailscale") || strings.HasPrefix(name, "zt") ||
			strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "awdl") ||
			strings.HasPrefix(name, "llw") || strings.HasPrefix(name, "bridge") {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || !isPrivate(ip) {
				continue
			}
			ones, _ := ipn.Mask.Size()
			if ones < 24 { // large networks: scan the /24 around this computer
				ones = 24
			}
			mask := net.CIDRMask(ones, 32)
			base := ip.Mask(mask)
			span := 1 << uint(32-ones)
			for i := 1; i < span-1 && i < 255; i++ {
				h := net.IPv4(base[0], base[1], base[2], base[3]+byte(i)).String()
				if h != ip.String() {
					add(h)
				}
			}
		}
	}
	return out
}

func isPrivate(ip net.IP) bool {
	return ip[0] == 10 || (ip[0] == 172 && ip[1]&0xf0 == 16) || (ip[0] == 192 && ip[1] == 168) ||
		(ip[0] == 169 && ip[1] == 254) || (ip[0] == 100 && ip[1]&0xc0 == 64)
}

// Drive is Vita storage (ux0:) mounted on this computer through VitaShell USB
// mode or a memory card reader.
type Drive struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Free int64  `json:"free"`
}

// LooksLikeVita reports whether dir is the root of a Vita ux0: partition.
func LooksLikeVita(dir string) bool {
	for _, n := range []string{"app", "data"} {
		st, err := os.Stat(filepath.Join(dir, n))
		if err != nil || !st.IsDir() {
			return false
		}
	}
	// ux0: also has at least one of these.
	for _, n := range []string{"id.dat", "license", "user", "appmeta", "VitaShell", "tai", "patch", "picture"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

// Drives lists mounted Vita storage.
func Drives() []Drive {
	var roots []string
	switch runtime.GOOS {
	case "windows":
		for c := 'D'; c <= 'Z'; c++ {
			roots = append(roots, string(c)+`:\`)
		}
	case "darwin":
		roots, _ = filepath.Glob("/Volumes/*")
	default:
		for _, p := range []string{"/media/*", "/media/*/*", "/run/media/*/*", "/mnt/*"} {
			m, _ := filepath.Glob(p)
			roots = append(roots, m...)
		}
	}
	var out []Drive
	for _, r := range roots {
		done := make(chan bool, 1)
		go func(r string) { done <- LooksLikeVita(r) }(r)
		select {
		case ok := <-done:
			if ok {
				name := filepath.Base(strings.TrimRight(r, `\/`))
				if runtime.GOOS == "windows" {
					name = "Drive " + r[:2]
				}
				out = append(out, Drive{Path: r, Name: name, Free: FreeSpace(r)})
			}
		case <-time.After(1500 * time.Millisecond): // a sleeping network or optical drive
		}
	}
	return out
}
