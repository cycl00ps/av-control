package eiscp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const DefaultPort = 60128

// Packet builds an eISCP frame for an ISCP body like "!1PWRQSTN\r".
func Packet(iscp string) []byte {
	if !strings.HasSuffix(iscp, "\r") {
		iscp += "\r"
	}
	body := []byte(iscp)
	hdr := make([]byte, 16)
	copy(hdr[0:4], []byte("ISCP"))
	binary.BigEndian.PutUint32(hdr[4:8], 16)
	binary.BigEndian.PutUint32(hdr[8:12], uint32(len(body)))
	hdr[12] = 1
	return append(hdr, body...)
}

// ParseMessages extracts !1CCCparam payloads from a buffer.
func ParseMessages(buf []byte) []Message {
	var out []Message
	s := string(buf)
	for {
		i := strings.Index(s, "!1")
		if i < 0 {
			break
		}
		s = s[i+2:]
		if len(s) < 3 {
			break
		}
		cmd := s[:3]
		rest := s[3:]
		end := len(rest)
		for j, c := range rest {
			if c == 0x1a || c == '\r' || c == '\n' {
				end = j
				break
			}
		}
		out = append(out, Message{Cmd: cmd, Param: rest[:end]})
		s = rest[end:]
	}
	return out
}

type Message struct {
	Cmd   string
	Param string
}

// Client is a single persistent eISCP TCP connection.
type Client struct {
	addr string
	mu   sync.Mutex
	conn net.Conn
}

func NewClient(host string, port int) *Client {
	if port == 0 {
		port = DefaultPort
	}
	return &Client{addr: fmt.Sprintf("%s:%d", host, port)}
}

func (c *Client) connect() error {
	if c.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// Query sends !1{cmd}QSTN and returns the matching response param (best effort).
func (c *Client) Query(cmd3 string) (string, error) {
	return c.Exchange(cmd3 + "QSTN", cmd3)
}

// Command sends !1{cmd}{param} and returns responses for mnemonic cmd3.
func (c *Client) Command(cmd3, param string) (string, error) {
	return c.Exchange(cmd3+param, cmd3)
}

func (c *Client) Exchange(bodyWithoutBang, expectCmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.connect(); err != nil {
		return "", err
	}
	pkt := Packet("!1" + bodyWithoutBang)
	_ = c.conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.conn.Write(pkt); err != nil {
		_ = c.conn.Close()
		c.conn = nil
		return "", err
	}
	time.Sleep(55 * time.Millisecond)
	buf := make([]byte, 0, 8192)
	tmp := make([]byte, 4096)
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, err := c.conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if strings.Contains(string(buf), "\x1a") {
				_ = c.conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
				for {
					n2, err2 := c.conn.Read(tmp)
					if n2 > 0 {
						buf = append(buf, tmp[:n2]...)
					}
					if err2 != nil {
						break
					}
				}
				break
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if len(buf) > 0 {
					break
				}
				continue
			}
			if err == io.EOF && len(buf) > 0 {
				break
			}
			_ = c.conn.Close()
			c.conn = nil
			if len(buf) == 0 {
				return "", err
			}
			break
		}
	}
	var last string
	for _, m := range ParseMessages(buf) {
		if m.Cmd == expectCmd {
			last = m.Param
		}
	}
	return last, nil
}
