package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/ftp"
)

// target is where files go: the Vita over FTP, or a mounted ux0: drive.
type target interface {
	// sizes lists the files in a Vita folder (name -> size); a missing folder is empty.
	sizes(ctx context.Context, worker int, dir string) (map[string]int64, bool, error)
	mkdirs(ctx context.Context, dirs []string) error
	put(ctx context.Context, worker int, local, remote string, progress func(int64)) error
	remove(remote string)
	exists(remote string) bool
	workers() int
	close()
}

// ---- FTP ----

type ftpTarget struct {
	addr  string
	mu    sync.Mutex
	conns []*ftp.Conn
	used  []bool // the worker has had a working connection
}

func newFTPTarget(ctx context.Context, addr string, n int) (*ftpTarget, error) {
	t := &ftpTarget{addr: addr, conns: make([]*ftp.Conn, n), used: make([]bool, n)}
	c, err := ftp.Dial(ctx, addr, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to the Vita at %s: %v", addr, err)
	}
	t.conns[0] = c
	t.used[0] = true
	return t, nil
}

func (t *ftpTarget) workers() int { return len(t.conns) }

// conn returns worker i's connection, opening it if needed.
func (t *ftpTarget) conn(ctx context.Context, i int) (*ftp.Conn, error) {
	t.mu.Lock()
	c := t.conns[i]
	t.mu.Unlock()
	if c != nil {
		return c, nil
	}
	c, err := ftp.Dial(ctx, t.addr, 15*time.Second)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.conns[i] = c
	t.used[i] = true
	t.mu.Unlock()
	return c, nil
}

func (t *ftpTarget) drop(i int) {
	t.mu.Lock()
	if t.conns[i] != nil {
		t.conns[i].Close()
		t.conns[i] = nil
	}
	t.mu.Unlock()
}

// retry runs fn on worker i's connection, reconnecting after network errors.
func (t *ftpTarget) retry(ctx context.Context, i int, fn func(*ftp.Conn) error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var c *ftp.Conn
		c, err = t.conn(ctx, i)
		t.mu.Lock()
		extra := i > 0 && !t.used[i]
		t.mu.Unlock()
		if err != nil && extra {
			return err // an extra connection the Vita refuses; the caller carries on without it
		}
		if err == nil {
			err = fn(c)
			var fe *ftp.Error
			if err == nil || errors.As(err, &fe) && fe.Code != 421 && fe.Code != 425 && fe.Code != 426 {
				return err
			}
		}
		t.drop(i)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-time.After(time.Duration(attempt+1) * 1500 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (t *ftpTarget) sizes(ctx context.Context, worker int, dir string) (map[string]int64, bool, error) {
	var entries []ftp.Entry
	missing := false
	err := t.retry(ctx, worker, func(c *ftp.Conn) error {
		var err error
		entries, err = c.List(dir)
		if ftp.IsNotFound(err) {
			missing = true
			return nil
		}
		return err
	})
	if err != nil {
		return nil, false, err
	}
	out := map[string]int64{}
	for _, e := range entries {
		if !e.IsDir {
			out[e.Name] = e.Size
		} else {
			out[e.Name+"/"] = -1
		}
	}
	return out, !missing, nil
}

func (t *ftpTarget) mkdirs(ctx context.Context, dirs []string) error {
	for _, d := range dirs {
		d := d
		if err := t.retry(ctx, 0, func(c *ftp.Conn) error { return c.Mkdir(d) }); err != nil {
			return fmt.Errorf("cannot create %s on the Vita: %v", d, err)
		}
	}
	return nil
}

func (t *ftpTarget) put(ctx context.Context, worker int, local, remote string, progress func(int64)) error {
	return t.retry(ctx, worker, func(c *ftp.Conn) error {
		f, err := os.Open(local)
		if err != nil {
			return err
		}
		defer f.Close()
		var sent int64
		err = c.Store(ctx, remote, f, func(n int64) { sent += n; progress(n) })
		if err != nil {
			progress(-sent) // this attempt's bytes will be sent again
		}
		return err
	})
}

func (t *ftpTarget) remove(remote string) {
	t.retry(context.Background(), 0, func(c *ftp.Conn) error { return c.Delete(remote) })
}

func (t *ftpTarget) exists(remote string) bool {
	ok := false
	t.retry(context.Background(), 0, func(c *ftp.Conn) error {
		var err error
		ok, err = c.Exists(remote)
		return err
	})
	return ok
}

// control runs fn on the first connection (for installation commands).
func (t *ftpTarget) control(ctx context.Context, fn func(*ftp.Conn) error) error {
	return t.retry(ctx, 0, fn)
}

func (t *ftpTarget) close() {
	for i := range t.conns {
		t.drop(i)
	}
}

// ---- Mounted drive ----

type driveTarget struct{ root string }

// local maps "ux0:data/x" to a path on the drive. Only ux0: is reachable.
func (d *driveTarget) local(remote string) (string, error) {
	if !strings.HasPrefix(remote, "ux0:") {
		return "", fmt.Errorf("%s is not on this drive", remote)
	}
	return filepath.Join(d.root, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(remote, "ux0:"), "/"))), nil
}

func (d *driveTarget) workers() int { return 2 }

func (d *driveTarget) sizes(_ context.Context, _ int, dir string) (map[string]int64, bool, error) {
	p, err := d.local(dir)
	if err != nil {
		return nil, false, err
	}
	entries, err := os.ReadDir(p)
	if os.IsNotExist(err) {
		return map[string]int64{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	out := map[string]int64{}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()+"/"] = -1
			continue
		}
		if info, err := e.Info(); err == nil {
			out[e.Name()] = info.Size()
		}
	}
	return out, true, nil
}

func (d *driveTarget) mkdirs(_ context.Context, dirs []string) error {
	for _, dir := range dirs {
		p, err := d.local(dir)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return fmt.Errorf("cannot create %s on the drive: %v", dir, err)
		}
	}
	return nil
}

func (d *driveTarget) put(ctx context.Context, _ int, local, remote string, progress func(int64)) error {
	dst, err := d.local(remote)
	if err != nil {
		return err
	}
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()
	os.MkdirAll(filepath.Dir(dst), 0o755)
	tmp := dst + ".cboc-part"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("cannot write to the drive: %v", err)
	}
	buf := make([]byte, 4<<20)
	var sent int64
	for {
		if ctx.Err() != nil {
			out.Close()
			os.Remove(tmp)
			progress(-sent)
			return ctx.Err()
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				out.Close()
				os.Remove(tmp)
				progress(-sent)
				return fmt.Errorf("cannot write to the drive (is it full?): %v", err)
			}
			sent += int64(n)
			progress(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			os.Remove(tmp)
			return rerr
		}
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	os.Remove(dst)
	return os.Rename(tmp, dst)
}

func (d *driveTarget) remove(remote string) {
	if p, err := d.local(remote); err == nil {
		os.Remove(p)
	}
}

func (d *driveTarget) exists(remote string) bool {
	p, err := d.local(remote)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func (d *driveTarget) close() {}

// parentDirs returns every folder that must exist for files, shallowest first.
func parentDirs(files []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		i := strings.IndexByte(f, ':')
		dev, rest := f[:i+1], f[i+1:]
		parts := strings.Split(path.Dir("/"+rest), "/")
		cur := dev
		for _, p := range parts {
			if p == "" {
				continue
			}
			if strings.HasSuffix(cur, ":") {
				cur += p
			} else {
				cur += "/" + p
			}
			if !seen[cur] {
				seen[cur] = true
				out = append(out, cur)
			}
		}
	}
	return out
}
