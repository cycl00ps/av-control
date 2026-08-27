package eiscp

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func TestPacketFrameLayout(t *testing.T) {
	p := Packet("!1PWRQSTN")
	if got := string(p[0:4]); got != "ISCP" {
		t.Fatalf("magic = %q, want ISCP", got)
	}
	if hl := binary.BigEndian.Uint32(p[4:8]); hl != 16 {
		t.Fatalf("header length = %d, want 16", hl)
	}
	body := "!1PWRQSTN\r"
	if bl := binary.BigEndian.Uint32(p[8:12]); bl != uint32(len(body)) {
		t.Fatalf("body length = %d, want %d", bl, len(body))
	}
	if p[12] != 1 {
		t.Fatalf("version = %d, want 1", p[12])
	}
	if got := string(p[16:]); got != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func TestPacketAppendsCarriageReturn(t *testing.T) {
	if !strings.HasSuffix(string(Packet("!1MVLQSTN")), "\r") {
		t.Fatal("packet body must end with \\r")
	}
	// Already-terminated bodies must not get a second \r.
	if got := string(Packet("!1MVLQSTN\r")[16:]); got != "!1MVLQSTN\r" {
		t.Fatalf("double CR: %q", got)
	}
}

func TestParseMessages(t *testing.T) {
	buf := append(Packet("!1PWR01\x1a")[16:], Packet("!1MVL42\r")[16:]...)
	msgs := ParseMessages(buf)
	if len(msgs) != 2 {
		t.Fatalf("parsed %d messages, want 2: %+v", len(msgs), msgs)
	}
	if msgs[0] != (Message{Cmd: "PWR", Param: "01"}) {
		t.Fatalf("msg0 = %+v", msgs[0])
	}
	if msgs[1] != (Message{Cmd: "MVL", Param: "42"}) {
		t.Fatalf("msg1 = %+v", msgs[1])
	}
}

func TestParseMessagesEmpty(t *testing.T) {
	if got := ParseMessages(nil); got != nil {
		t.Fatalf("ParseMessages(nil) = %+v, want nil", got)
	}
	if got := ParseMessages([]byte("garbage without frames")); got != nil {
		t.Fatalf("ParseMessages(garbage) = %+v, want nil", got)
	}
}

func TestQueryAgainstFakeDevice(t *testing.T) {
	fs := newFakeServer(t)
	fs.reply["PWR"] = "01"
	fs.reply["MVL"] = "37"

	host, port := fs.Addr()
	c := NewClient(host, port)
	defer c.Close()

	if v, err := c.Query("PWR"); err != nil || v != "01" {
		t.Fatalf("Query(PWR) = %q, %v; want 01, nil", v, err)
	}
	if v, err := c.Query("MVL"); err != nil || v != "37" {
		t.Fatalf("Query(MVL) = %q, %v; want 37, nil", v, err)
	}
}

func TestCommandSendsParam(t *testing.T) {
	fs := newFakeServer(t)
	fs.reply["MVL"] = "50"

	host, port := fs.Addr()
	c := NewClient(host, port)
	defer c.Close()

	v, err := c.Command("MVL", "50")
	if err != nil || v != "50" {
		t.Fatalf("Command(MVL,50) = %q, %v; want 50, nil", v, err)
	}
}

func TestExchangeAllCollectsFrames(t *testing.T) {
	fs := newFakeServer(t)
	fs.reply["NLS"] = "U0-Top"
	fs.extra = []string{"!1NLSU1-Second\r", "!1NLSU2-Third\r"}

	host, port := fs.Addr()
	c := NewClient(host, port)
	defer c.Close()

	msgs, err := c.ExchangeAll("NLSQSTN", 800*time.Millisecond)
	if err != nil {
		t.Fatalf("ExchangeAll: %v", err)
	}
	var lines []string
	for _, m := range msgs {
		if m.Cmd == "NLS" {
			lines = append(lines, m.Param)
		}
	}
	if len(lines) < 2 {
		t.Fatalf("ExchangeAll got %d NLS frames, want >=2: %v", len(lines), lines)
	}
}

func TestQueryConnError(t *testing.T) {
	// Port on loopback with nothing listening: dial must fail cleanly.
	c := NewClient("127.0.0.1", 1)
	if _, err := c.Query("PWR"); err == nil {
		t.Fatal("expected dial error against closed port")
	}
}

func TestCloseIdempotent(t *testing.T) {
	fs := newFakeServer(t)
	host, port := fs.Addr()
	c := NewClient(host, port)
	if _, err := c.Query("PWR"); err != nil {
		t.Fatalf("query before close: %v", err)
	}
	c.Close()
	c.Close() // must not panic
}
