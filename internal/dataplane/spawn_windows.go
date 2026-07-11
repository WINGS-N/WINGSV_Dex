//go:build windows

package dataplane

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// startHelper elevates a copy of the app as the net-helper (UAC prompt) and talks to it
// over a named pipe. Windows cannot pipe stdio across the elevation boundary, so the
// (medium-integrity) GUI hosts the control pipe and the (high-integrity) helper connects
// to it as a client; a higher-integrity client may open a lower-integrity pipe.
func startHelper(exePath string, logw io.Writer) (*helper, error) {
	logLine(logw, "dataplane: launching elevated helper exe=%s", exePath)
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	pipeName := `\\.\pipe\wingsv-dex-` + hex.EncodeToString(b[:])

	l, err := winio.ListenPipe(pipeName, &winio.PipeConfig{})
	if err != nil {
		return nil, fmt.Errorf("dataplane: create control pipe: %w", err)
	}

	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exePath)
	args, _ := windows.UTF16PtrFromString("--net-helper --pipe " + pipeName)
	if err := windows.ShellExecute(0, verb, file, args, nil, windows.SW_HIDE); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("dataplane: elevate helper: %w", err)
	}
	logLine(logw, "dataplane: waiting for elevated helper pipe=%s", pipeName)

	// Wait for the helper to connect, but give up if the user denied UAC or it never
	// comes up. Closing the listener unblocks the pending Accept.
	type accepted struct {
		c net.Conn
		e error
	}
	ch := make(chan accepted, 1)
	go func() { c, e := l.Accept(); ch <- accepted{c, e} }()

	var conn net.Conn
	select {
	case r := <-ch:
		if r.e != nil {
			_ = l.Close()
			return nil, fmt.Errorf("dataplane: helper connect: %w", r.e)
		}
		conn = r.c
		logLine(logw, "dataplane: elevated helper connected")
	case <-time.After(60 * time.Second):
		_ = l.Close()
		return nil, fmt.Errorf("dataplane: timed out waiting for the elevated helper (authorization denied?)")
	}

	return &helper{
		enc: json.NewEncoder(conn),
		dec: json.NewDecoder(conn),
		stop: func() {
			_ = conn.Close()
			_ = l.Close()
		},
	}, nil
}
