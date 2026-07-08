//go:build linux

package wg

import (
	"log"
	"net"
	"strings"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// ProtectServer hosts the abstract unix socket vkturn dials as its -protect-sock.
// vkturn passes each outbound socket fd over it (SCM_RIGHTS); the server stamps the
// fd with SO_MARK = fwmark so the fwmark bypass rule keeps that underlay traffic off
// the tunnel (the Linux equivalent of Android's VpnService.protect). Runs privileged
// (SO_MARK needs CAP_NET_ADMIN).
type ProtectServer struct {
	ln     *net.UnixListener
	fwmark int
	marked atomic.Int64
}

// ListenProtect starts the protect server on the given abstract socket name (a "@"
// prefix is added if absent, matching vkturn's client side).
func ListenProtect(abstractName string, fwmark int) (*ProtectServer, error) {
	name := strings.TrimSpace(abstractName)
	if !strings.HasPrefix(name, "@") {
		name = "@" + name
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: name, Net: "unix"})
	if err != nil {
		return nil, err
	}
	s := &ProtectServer{ln: ln, fwmark: fwmark}
	go s.acceptLoop()
	return s, nil
}

func (s *ProtectServer) acceptLoop() {
	for {
		conn, err := s.ln.AcceptUnix()
		if err != nil {
			return
		}
		log.Printf("net-helper: protect: vkturn connected")
		go s.handle(conn)
	}
}

func (s *ProtectServer) handle(conn *net.UnixConn) {
	defer conn.Close()
	msg := make([]byte, 1)
	oob := make([]byte, 128)
	for {
		_, oobn, _, _, err := conn.ReadMsgUnix(msg, oob)
		if err != nil {
			return
		}
		ack := byte(1)
		if oobn > 0 {
			if !s.markFDs(oob[:oobn]) {
				ack = 0
			}
		}
		if _, err := conn.Write([]byte{ack}); err != nil {
			return
		}
	}
}

// markFDs stamps every fd carried in the control message with SO_MARK and closes
// this process's copy. Returns false if any fd could not be marked.
func (s *ProtectServer) markFDs(oob []byte) bool {
	scms, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return false
	}
	ok := true
	for i := range scms {
		fds, err := unix.ParseUnixRights(&scms[i])
		if err != nil {
			ok = false
			continue
		}
		for _, fd := range fds {
			if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_MARK, s.fwmark); err != nil {
				ok = false
				log.Printf("net-helper: protect: SO_MARK failed: %v", err)
			} else if n := s.marked.Add(1); n <= 3 || n%25 == 0 {
				log.Printf("net-helper: protect: marked %d socket(s) with fwmark 0x%x", n, s.fwmark)
			}
			_ = unix.Close(fd)
		}
	}
	return ok
}

// Close stops accepting and removes the socket.
func (s *ProtectServer) Close() error {
	return s.ln.Close()
}
