package eiscp

import (
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
)

// fakeServer is an in-process eISCP device stand-in: it speaks the ISCP
// frame protocol over TCP so tests never need a real AVR on the LAN.
type fakeServer struct {
	ln    net.Listener
	mu    sync.Mutex
	conns []net.Conn
	// reply maps a 3-char mnemonic to the parameter the fake answers with.
	reply map[string]string
	// extra frames appended (as raw ISCP bodies) after the main reply.
	extra []string
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	fs := &fakeServer{ln: ln, reply: map[string]string{}}
	t.Cleanup(fs.close)
	go fs.serve()
	return fs
}

// Addr returns host and port for NewClient.
func (fs *fakeServer) Addr() (string, int) {
	tc := fs.ln.Addr().(*net.TCPAddr)
	return tc.IP.String(), tc.Port
}

func (fs *fakeServer) close() { _ = fs.ln.Close() }

func (fs *fakeServer) serve() {
	for {
		conn, err := fs.ln.Accept()
		if err != nil {
			return
		}
		fs.mu.Lock()
		fs.conns = append(fs.conns, conn)
		fs.mu.Unlock()
		go fs.handle(conn)
	}
}

func (fs *fakeServer) handle(conn net.Conn) {
	defer conn.Close()
	hdr := make([]byte, 16)
	for {
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return
		}
		if string(hdr[0:4]) != "ISCP" {
			return
		}
		hdrLen := binary.BigEndian.Uint32(hdr[4:8])
		bodyLen := binary.BigEndian.Uint32(hdr[8:12])
		body := make([]byte, bodyLen)
		if _, err := io.ReadFull(conn, body); err != nil {
			return
		}
		_ = hdrLen
		req := strings.TrimRight(string(body), "\r")
		fs.replyTo(conn, req)
	}
}

// replyTo answers "!1PWRQSTN" style requests with a terminating frame
// (0x1a) plus any configured extra frames.
func (fs *fakeServer) replyTo(conn net.Conn, req string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(req) < 5 || req[0] != '!' {
		return
	}
	cmd := req[2:5]
	param, ok := fs.reply[cmd]
	if !ok {
		param = "01"
	}
	frame := "!1" + cmd + param + "\x1a"
	_, _ = conn.Write(Packet(frame))
	for _, x := range fs.extra {
		_, _ = conn.Write(Packet(x))
	}
}
