// Package ftp is a small FTP client tuned for the FTP servers that run on a
// PS Vita (VitaShell and Vita Companion). Only passive mode is used.
package ftp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Error is a negative reply from the server.
type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("%d %s", e.Code, e.Msg) }

// IsNotFound reports whether err is a "file unavailable" reply.
func IsNotFound(err error) bool {
	var fe *Error
	return errors.As(err, &fe) && fe.Code == 550
}

// Entry is one line of a directory listing.
type Entry struct {
	Name  string
	Size  int64
	IsDir bool
}

// Conn is one control connection.
type Conn struct {
	host    string
	conn    net.Conn
	r       *bufio.Reader
	Timeout time.Duration
	Banner  string
}

// Dial connects and logs in anonymously.
func Dial(ctx context.Context, addr string, timeout time.Duration) (*Conn, error) {
	d := net.Dialer{Timeout: timeout}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	host, _, _ := net.SplitHostPort(addr)
	c := &Conn{host: host, conn: nc, r: bufio.NewReaderSize(nc, 64<<10), Timeout: timeout}
	if tcp, ok := nc.(*net.TCPConn); ok {
		tcp.SetNoDelay(true)
	}
	code, msg, err := c.read()
	if err != nil {
		nc.Close()
		return nil, err
	}
	if code != 220 {
		nc.Close()
		return nil, &Error{code, msg}
	}
	c.Banner = msg
	code, msg, err = c.Cmd("USER anonymous")
	if err == nil && code == 331 {
		code, msg, err = c.Cmd("PASS anonymous@")
	}
	if err != nil {
		nc.Close()
		return nil, err
	}
	if code != 230 && code != 202 {
		nc.Close()
		return nil, &Error{code, msg}
	}
	if code, msg, err = c.Cmd("TYPE I"); err != nil || code >= 400 {
		nc.Close()
		if err == nil {
			err = &Error{code, msg}
		}
		return nil, err
	}
	return c, nil
}

// Close ends the session.
func (c *Conn) Close() error {
	c.conn.SetDeadline(time.Now().Add(time.Second))
	fmt.Fprintf(c.conn, "QUIT\r\n")
	return c.conn.Close()
}

func (c *Conn) read() (int, string, error) {
	c.conn.SetReadDeadline(time.Now().Add(c.Timeout))
	line, err := c.r.ReadString('\n')
	if err != nil {
		return 0, "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) < 3 {
		return 0, "", fmt.Errorf("bad FTP reply %q", line)
	}
	code, err := strconv.Atoi(line[:3])
	if err != nil {
		return 0, "", fmt.Errorf("bad FTP reply %q", line)
	}
	msg := strings.TrimSpace(line[3:])
	if len(line) > 3 && line[3] == '-' {
		end := line[:3] + " "
		for {
			l, err := c.r.ReadString('\n')
			if err != nil {
				return 0, "", err
			}
			l = strings.TrimRight(l, "\r\n")
			if strings.HasPrefix(l, end) {
				msg += "\n" + strings.TrimSpace(l[4:])
				break
			}
			msg += "\n" + strings.TrimSpace(l)
		}
	}
	return code, msg, nil
}

// Cmd sends one command and returns the reply.
func (c *Conn) Cmd(cmd string) (int, string, error) {
	c.conn.SetWriteDeadline(time.Now().Add(c.Timeout))
	if _, err := io.WriteString(c.conn, cmd+"\r\n"); err != nil {
		return 0, "", err
	}
	return c.read()
}

// CmdWait is Cmd with a longer reply deadline (used for installation).
func (c *Conn) CmdWait(cmd string, wait time.Duration) (int, string, error) {
	old := c.Timeout
	c.Timeout = wait
	defer func() { c.Timeout = old }()
	return c.Cmd(cmd)
}

func (c *Conn) expect(cmd string, ok ...int) error {
	code, msg, err := c.Cmd(cmd)
	if err != nil {
		return err
	}
	for _, o := range ok {
		if code == o {
			return nil
		}
	}
	return &Error{code, msg}
}

func (c *Conn) pasv() (net.Conn, error) {
	code, msg, err := c.Cmd("PASV")
	if err != nil {
		return nil, err
	}
	if code != 227 {
		return nil, &Error{code, msg}
	}
	open, close := strings.IndexByte(msg, '('), strings.IndexByte(msg, ')')
	if open < 0 || close < open {
		return nil, fmt.Errorf("bad PASV reply %q", msg)
	}
	parts := strings.Split(msg[open+1:close], ",")
	if len(parts) != 6 {
		return nil, fmt.Errorf("bad PASV reply %q", msg)
	}
	hi, _ := strconv.Atoi(strings.TrimSpace(parts[4]))
	lo, _ := strconv.Atoi(strings.TrimSpace(parts[5]))
	// Always use the control connection's address: the Vita sometimes
	// reports an address the computer cannot reach.
	dc, err := net.DialTimeout("tcp", net.JoinHostPort(c.host, strconv.Itoa(hi<<8|lo)), c.Timeout)
	if err != nil {
		return nil, err
	}
	if tcp, ok := dc.(*net.TCPConn); ok {
		tcp.SetWriteBuffer(1 << 20)
		tcp.SetReadBuffer(1 << 20)
	}
	return dc, nil
}

// Path converts "ux0:data/x" to the "/ux0:/data/x" form Vita servers use.
func Path(vitaPath string) string {
	i := strings.IndexByte(vitaPath, ':')
	if i < 0 {
		return vitaPath
	}
	return "/" + vitaPath[:i+1] + "/" + strings.TrimPrefix(vitaPath[i+1:], "/")
}

// Cwd changes directory.
func (c *Conn) Cwd(dir string) error { return c.expect("CWD "+Path(dir), 250, 200) }

// Mkdir creates one directory. An existing directory is not an error.
func (c *Conn) Mkdir(dir string) error {
	err := c.expect("MKD "+Path(dir), 257, 250, 200)
	if err != nil && IsNotFound(err) {
		if c.Cwd(dir) == nil {
			return nil
		}
	}
	return err
}

// Delete removes a file.
func (c *Conn) Delete(path string) error { return c.expect("DELE "+Path(path), 250, 200) }

// Rmdir removes an empty directory.
func (c *Conn) Rmdir(path string) error { return c.expect("RMD "+Path(path), 250, 200) }

// Rename moves a file or directory.
func (c *Conn) Rename(from, to string) error {
	if err := c.expect("RNFR "+Path(from), 350); err != nil {
		return err
	}
	return c.expect("RNTO "+Path(to), 250, 200)
}

// List returns the entries in dir. A missing directory gives IsNotFound.
func (c *Conn) List(dir string) ([]Entry, error) {
	if err := c.Cwd(dir); err != nil {
		return nil, err
	}
	dc, err := c.pasv()
	if err != nil {
		return nil, err
	}
	defer dc.Close()
	code, msg, err := c.Cmd("LIST")
	if err != nil {
		return nil, err
	}
	if code != 150 && code != 125 {
		return nil, &Error{code, msg}
	}
	dc.SetReadDeadline(time.Now().Add(c.Timeout * 2))
	data, err := io.ReadAll(io.LimitReader(dc, 16<<20))
	dc.Close()
	if err != nil {
		return nil, err
	}
	if code, msg, err = c.read(); err != nil {
		return nil, err
	} else if code >= 400 {
		return nil, &Error{code, msg}
	}
	return parseList(string(data)), nil
}

func parseList(data string) []Entry {
	var out []Entry
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}
		// "drwxrwxrwx 1 vita vita 0 Jan  1 00:00 name with spaces"
		rest := line
		fields := make([]string, 0, 8)
		for len(fields) < 8 {
			rest = strings.TrimLeft(rest, " ")
			i := strings.IndexByte(rest, ' ')
			if i < 0 {
				break
			}
			fields = append(fields, rest[:i])
			rest = rest[i+1:]
		}
		if len(fields) < 8 || rest == "" {
			continue
		}
		name := strings.TrimLeft(rest, " ")
		if name == "." || name == ".." {
			continue
		}
		size, _ := strconv.ParseInt(fields[4], 10, 64)
		out = append(out, Entry{Name: name, Size: size, IsDir: strings.HasPrefix(fields[0], "d")})
	}
	return out
}

// Size returns a file's size. ok is false when the file does not exist.
func (c *Conn) Size(path string) (size int64, ok bool, err error) {
	code, msg, err := c.Cmd("SIZE " + Path(path))
	if err != nil {
		return 0, false, err
	}
	if code == 213 {
		n, err := strconv.ParseInt(strings.TrimSpace(msg), 10, 64)
		return n, err == nil, nil
	}
	if code == 550 {
		return 0, false, nil
	}
	// SIZE unsupported: fall back to listing the parent directory.
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return 0, false, &Error{code, msg}
	}
	entries, err := c.List(path[:i])
	if IsNotFound(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	for _, e := range entries {
		if e.Name == path[i+1:] && !e.IsDir {
			return e.Size, true, nil
		}
	}
	return 0, false, nil
}

// Exists reports whether a file or directory exists.
func (c *Conn) Exists(path string) (bool, error) {
	if err := c.Cwd(path); err == nil {
		return true, nil
	}
	_, ok, err := c.Size(path)
	return ok, err
}

// Store uploads r to path. progress receives byte counts as they are sent.
func (c *Conn) Store(ctx context.Context, path string, r io.Reader, progress func(int64)) error {
	dc, err := c.pasv()
	if err != nil {
		return err
	}
	defer dc.Close()
	code, msg, err := c.Cmd("STOR " + Path(path))
	if err != nil {
		return err
	}
	if code != 150 && code != 125 {
		return &Error{code, msg}
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			dc.Close()
			c.conn.Close()
		case <-stop:
		}
	}()
	buf := make([]byte, 512<<10)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			dc.SetWriteDeadline(time.Now().Add(c.Timeout * 2))
			if _, err := dc.Write(buf[:n]); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			if progress != nil {
				progress(int64(n))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := dc.Close(); err != nil {
		return err
	}
	// The Vita may need a while to flush a large file to its card.
	code, msg, err = c.CmdWaitReply(2 * time.Minute)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if code >= 400 {
		return &Error{code, msg}
	}
	return nil
}

// CmdWaitReply reads one more reply with a custom deadline.
func (c *Conn) CmdWaitReply(wait time.Duration) (int, string, error) {
	old := c.Timeout
	c.Timeout = wait
	defer func() { c.Timeout = old }()
	return c.read()
}

// Retrieve downloads up to limit bytes of a file.
func (c *Conn) Retrieve(path string, limit int64) ([]byte, error) {
	dc, err := c.pasv()
	if err != nil {
		return nil, err
	}
	defer dc.Close()
	code, msg, err := c.Cmd("RETR " + Path(path))
	if err != nil {
		return nil, err
	}
	if code != 150 && code != 125 {
		return nil, &Error{code, msg}
	}
	dc.SetReadDeadline(time.Now().Add(c.Timeout * 4))
	data, err := io.ReadAll(io.LimitReader(dc, limit))
	dc.Close()
	if err != nil {
		return nil, err
	}
	// Stopping early can make the server report an aborted transfer; that is fine.
	c.read()
	return data, nil
}
