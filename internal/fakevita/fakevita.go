// Package fakevita is a pretend PS Vita for tests: an FTP server that behaves
// like VitaShell's (paths such as /ux0:/data, PASV only, PROM installs a VPK)
// and an optional Vita Companion command port.
package fakevita

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/vita"
)

// Vita is a running fake console whose storage lives under Root/ux0 and Root/ur0.
type Vita struct {
	Root       string
	FTP        net.Listener
	Companion  net.Listener
	PROM       bool // VitaShell-style remote install
	Stores     int32
	MaxClients int32
	clients    int32
	Launched   []string
	mu         sync.Mutex
	// Speed limits each upload to this many bytes per second (0 = unlimited),
	// roughly like a Vita on Wi-Fi.
	Speed int64
	// Limit refuses connections beyond this many at once (0 = no limit).
	Limit int32
	// FailStoreAfter makes the n-th STOR drop the connection mid-file (once).
	FailStoreAfter int32
	// OddMkd answers MKD with "226 Directory created." like some Vita FTP servers.
	OddMkd bool
}

// Start runs a fake Vita. companion adds the 1338-style command port.
func Start(root string, prom, companion bool) (*Vita, error) {
	for _, d := range []string{"ux0/app", "ux0/data", "ux0/license", "ur0/data"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	v := &Vita{Root: root, FTP: l, PROM: prom}
	go v.serveFTP()
	if companion {
		c, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		v.Companion = c
		go v.serveCompanion()
	}
	return v, nil
}

// Port is the FTP port.
func (v *Vita) Port() int { return v.FTP.Addr().(*net.TCPAddr).Port }

// CompanionPort is the command port, or 0.
func (v *Vita) CompanionPort() int {
	if v.Companion == nil {
		return 0
	}
	return v.Companion.Addr().(*net.TCPAddr).Port
}

// Close stops the fake.
func (v *Vita) Close() {
	v.FTP.Close()
	if v.Companion != nil {
		v.Companion.Close()
	}
}

func (v *Vita) local(p string) (string, bool) {
	p = strings.TrimPrefix(p, "/")
	i := strings.Index(p, ":")
	if i < 0 {
		return "", false
	}
	dev := p[:i]
	if dev != "ux0" && dev != "ur0" {
		return "", false
	}
	rest := strings.TrimPrefix(p[i+1:], "/")
	for _, part := range strings.Split(rest, "/") {
		if part == ".." {
			return "", false
		}
	}
	return filepath.Join(v.Root, dev, filepath.FromSlash(rest)), true
}

func (v *Vita) serveFTP() {
	for {
		c, err := v.FTP.Accept()
		if err != nil {
			return
		}
		go v.session(c)
	}
}

func (v *Vita) session(c net.Conn) {
	n := atomic.AddInt32(&v.clients, 1)
	for {
		m := atomic.LoadInt32(&v.MaxClients)
		if n <= m || atomic.CompareAndSwapInt32(&v.MaxClients, m, n) {
			break
		}
	}
	defer atomic.AddInt32(&v.clients, -1)
	defer c.Close()
	r := bufio.NewReader(c)
	say := func(f string, a ...interface{}) { fmt.Fprintf(c, f+"\r\n", a...) }
	if l := atomic.LoadInt32(&v.Limit); l > 0 && n > l {
		say("421 Too many connections.")
		return
	}
	say("220 FTPVita Server ready.")
	cwd := "/"
	var pasv net.Listener
	var rnfr string
	abs := func(arg string) string {
		if strings.HasPrefix(arg, "/") {
			return arg
		}
		return strings.TrimSuffix(cwd, "/") + "/" + arg
	}
	data := func() net.Conn {
		if pasv == nil {
			return nil
		}
		d, err := pasv.Accept()
		pasv.Close()
		pasv = nil
		if err != nil {
			return nil
		}
		return d
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd, arg := line, ""
		if i := strings.IndexByte(line, ' '); i >= 0 {
			cmd, arg = line[:i], line[i+1:]
		}
		switch strings.ToUpper(cmd) {
		case "USER":
			say("331 Username OK, need password b0ss.")
		case "PASS":
			say("230 User logged in!")
		case "TYPE":
			say("200 Switching to Binary Mode")
		case "PASV":
			l, _ := net.Listen("tcp", "127.0.0.1:0")
			pasv = l
			p := l.Addr().(*net.TCPAddr).Port
			// Like a real Vita, report an address that may not be reachable.
			say("227 Entering Passive Mode (10,9,8,7,%d,%d)", p>>8, p&255)
		case "CWD":
			lp, ok := v.local(abs(arg))
			if st, err := os.Stat(lp); ok && err == nil && st.IsDir() {
				cwd = abs(arg)
				say("250 Requested file action okay, completed.")
			} else {
				say("550 Invalid directory.")
			}
		case "LIST":
			d := data()
			if d == nil {
				say("425 No data connection")
				continue
			}
			say("150 Opening ASCII mode data transfer for LIST.")
			lp, _ := v.local(cwd)
			entries, _ := os.ReadDir(lp)
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			for _, e := range entries {
				info, _ := e.Info()
				perm := "-rw-rw-rw-"
				var size int64
				if e.IsDir() {
					perm = "drwxrwxrwx"
				} else {
					size = info.Size()
				}
				fmt.Fprintf(d, "%s 1 vita vita %d Jan  1 00:00 %s\r\n", perm, size, e.Name())
			}
			d.Close()
			say("226 Transfer complete.")
		case "SIZE":
			lp, ok := v.local(abs(arg))
			st, err := os.Stat(lp)
			if !ok || err != nil || st.IsDir() {
				say("550 The file doesn't exist.")
			} else {
				say("213 %d", st.Size())
			}
		case "MKD":
			lp, ok := v.local(abs(arg))
			if _, err := os.Stat(filepath.Dir(lp)); !ok || err != nil {
				say("550 Could not create the directory.")
				continue
			}
			if os.Mkdir(lp, 0o755) != nil {
				say("550 Could not create the directory.")
			} else {
				if v.OddMkd {
					say("226 Directory created.")
				} else {
					say("257 \"%s\" created.", arg)
				}
			}
		case "STOR":
			lp, ok := v.local(abs(arg))
			d := data()
			if !ok || d == nil {
				say("550 Could not open file.")
				continue
			}
			f, err := os.Create(lp)
			if err != nil {
				d.Close()
				say("550 Could not open file.")
				continue
			}
			say("150 Opening Image mode data transfer.")
			n := atomic.AddInt32(&v.Stores, 1)
			if fa := atomic.LoadInt32(&v.FailStoreAfter); fa > 0 && n == fa {
				atomic.StoreInt32(&v.FailStoreAfter, 0)
				io.CopyN(f, d, 1000)
				f.Close()
				d.Close()
				return // drop the whole session mid-file
			}
			if v.Speed > 0 {
				buf := make([]byte, 32<<10)
				for {
					n, err := d.Read(buf)
					f.Write(buf[:n])
					if err != nil {
						break
					}
					time.Sleep(time.Duration(int64(n) * int64(time.Second) / v.Speed))
				}
			} else {
				io.Copy(f, d)
			}
			f.Close()
			d.Close()
			say("226 Transfer complete.")
		case "RETR":
			lp, ok := v.local(abs(arg))
			f, err := os.Open(lp)
			d := data()
			if !ok || err != nil || d == nil {
				if d != nil {
					d.Close()
				}
				say("550 Could not open file.")
				continue
			}
			say("150 Opening Image mode data transfer.")
			io.Copy(d, f)
			f.Close()
			d.Close()
			say("226 Transfer complete.")
		case "DELE":
			lp, ok := v.local(abs(arg))
			if ok && os.Remove(lp) == nil {
				say("226 File deleted.")
			} else {
				say("550 Could not delete the file.")
			}
		case "RMD":
			lp, ok := v.local(abs(arg))
			if ok && os.Remove(lp) == nil {
				say("226 Directory deleted.")
			} else {
				say("550 Could not delete the directory.")
			}
		case "RNFR":
			rnfr = abs(arg)
			say("350 Remember to send RNTO")
		case "RNTO":
			a, ok1 := v.local(rnfr)
			b, ok2 := v.local(abs(arg))
			if ok1 && ok2 && os.Rename(a, b) == nil {
				say("226 Rename completed.")
			} else {
				say("550 Error renaming the file.")
			}
		case "PROM":
			if !v.PROM {
				say("502 Command not implemented.")
				continue
			}
			lp, ok := v.local("/" + strings.Replace(arg, ":", ":/", 1))
			if !ok || v.installVPK(lp) != nil {
				say("500 ERROR PROMOTING")
			} else {
				say("200 OK PROMOTING")
			}
		case "QUIT":
			say("221 Goodbye senpai :'(")
			return
		default:
			say("502 Command not implemented.")
		}
	}
}

// installVPK "promotes" a VPK by unzipping it to ux0:app/TITLEID.
func (v *Vita) installVPK(vpk string) error {
	z, err := zip.OpenReader(vpk)
	if err != nil {
		return err
	}
	defer z.Close()
	id := ""
	for _, f := range z.File {
		if f.Name == "sce_sys/param.sfo" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			id = titleFromSFO(b)
		}
	}
	if id == "" {
		return fmt.Errorf("no title")
	}
	dest := filepath.Join(v.Root, "ux0", "app", id)
	os.RemoveAll(dest)
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		p := filepath.Join(dest, filepath.FromSlash(f.Name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		rc, _ := f.Open()
		out, _ := os.Create(p)
		io.Copy(out, rc)
		out.Close()
		rc.Close()
	}
	return nil
}

func titleFromSFO(b []byte) string {
	m, err := vita.ParseSFO(b)
	if err != nil {
		return ""
	}
	return m["TITLE_ID"]
}

func (v *Vita) serveCompanion() {
	for {
		c, err := v.Companion.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			line, _ := bufio.NewReader(c).ReadString('\n')
			f := strings.Fields(line)
			if len(f) == 0 {
				return
			}
			switch f[0] {
			case "version":
				fmt.Fprint(c, "Vita Companion v2.1\n")
			case "nosleep":
				fmt.Fprint(c, "OK\n")
			case "launch":
				v.mu.Lock()
				v.Launched = append(v.Launched, f[1])
				v.mu.Unlock()
				fmt.Fprint(c, "Launched.\n")
			case "promote":
				lp, ok := v.local("/" + strings.Replace(f[1], ":", ":/", 1))
				b, err := os.ReadFile(filepath.Join(lp, "sce_sys", "param.sfo"))
				if !ok || err != nil {
					fmt.Fprint(c, "Error: Directory does not contain a promotable Vita application.\n")
					return
				}
				id := titleFromSFO(b)
				dest := filepath.Join(v.Root, "ux0", "app", id)
				os.RemoveAll(dest)
				if os.Rename(lp, dest) != nil {
					fmt.Fprint(c, "Error: Could not promote directory (0x80010001).\n")
					return
				}
				os.Mkdir(lp, 0o755) // the real promoter leaves the folder behind
				fmt.Fprint(c, "Promoted.\n")
			default:
				fmt.Fprint(c, "Error: Unknown command\n")
			}
		}(c)
	}
}
