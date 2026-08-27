package state

import (
	"math"
	"testing"
)

func TestHexVolToDisplay(t *testing.T) {
	cases := []struct {
		hex  string
		want float64
	}{
		{"00", 0},
		{"50", 40},
		{"64", 50},
		{" C8 ", 100},
		{"ZZ", 40}, // invalid → default 40
		{"", 40},
	}
	for _, c := range cases {
		if got := HexVolToDisplay(c.hex); got != c.want {
			t.Errorf("HexVolToDisplay(%q) = %v, want %v", c.hex, got, c.want)
		}
	}
}

func TestDisplayToHexVol(t *testing.T) {
	cases := []struct {
		d    float64
		want string
	}{
		{0, "00"},
		{40, "50"},
		{100, "C8"},
		{-5, "00"},   // clamped low
		{150, "C8"},  // clamped high (200 half-steps)
	}
	for _, c := range cases {
		if got := DisplayToHexVol(c.d); got != c.want {
			t.Errorf("DisplayToHexVol(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestCTLWireRoundTrip(t *testing.T) {
	cases := []struct {
		wire string
		db   float64
	}{
		{"000", 0},
		{"+14", 10},
		{"+0A", 5},
		{"-01", -0.5},
		{"-18", -12},
	}
	for _, c := range cases {
		db, ok := CTLWireToDb(c.wire)
		if !ok || db != c.db {
			t.Errorf("CTLWireToDb(%q) = %v,%v want %v,true", c.wire, db, ok, c.db)
		}
		if got := FormatCTLParam(c.db); got != c.wire {
			t.Errorf("FormatCTLParam(%v) = %q, want %q", c.db, got, c.wire)
		}
	}
}

func TestCTLWireInvalid(t *testing.T) {
	for _, bad := range []string{"", "QSTN", "UP", "DOWN", "N/A", "xyz"} {
		if _, ok := CTLWireToDb(bad); ok {
			t.Errorf("CTLWireToDb(%q) = ok, want false", bad)
		}
	}
}

func TestParseCTLEnv(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"+10", 10},
		{"-1.5", -1.5},
		{"", 0},
		{"junk", 0},
		{"+50", 12},  // clamped
		{"-50", -12}, // clamped
	}
	for _, c := range cases {
		if got := ParseCTLEnv(c.in); got != c.want {
			t.Errorf("ParseCTLEnv(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseIFA(t *testing.T) {
	in, out, hint := ParseIFA("HDMI 4,DTS-HD,48 kHz,5.1 ch,Straight Decode,3.1 ch,")
	if in == "" || out == "" {
		t.Fatalf("ParseIFA in/out empty: %q %q", in, out)
	}
	if hint != "Processed" {
		t.Fatalf("hint = %q, want Processed (5.1 in vs 3.1 out)", hint)
	}
	// Pass-through case: same channel counts.
	_, _, hint = ParseIFA("HDMI 4,PCM,48 kHz,2.0 ch,Straight Decode,2.0 ch,")
	if hint != "Pass-through" {
		t.Fatalf("hint = %q, want Pass-through", hint)
	}
}

func TestParseIFV(t *testing.T) {
	in, out, hint := ParseIFV("HDMI 4,4K(3840x2160) 59 Hz,YCbCr422,24bit,MAIN,1080p 60 Hz,YCbCr422,24bit,,,")
	if in == "" || out == "" {
		t.Fatalf("ParseIFV in/out empty: %q %q", in, out)
	}
	if hint != "Processed" {
		t.Fatalf("hint = %q, want Processed (4K in vs 1080p out)", hint)
	}
}

func TestParseFormatTUN(t *testing.T) {
	// FM: 5 digits = MHz*100.
	fm, am, ok := ParseTUN("10110", false)
	if !ok || fm != 101.10 || am != 0 {
		t.Fatalf("ParseTUN(10110) = %v,%v,%v want 101.10,0,true", fm, am, ok)
	}
	// AM: value <= 1710 treated as kHz.
	fm, am, ok = ParseTUN("0810", false)
	if !ok || am != 810 || fm != 0 {
		t.Fatalf("ParseTUN(0810) = %v,%v,%v want 0,810,true", fm, am, ok)
	}
	// Invalid params.
	for _, bad := range []string{"", "N/A", "QSTN", "UP", "DOWN", "abc"} {
		if _, _, ok := ParseTUN(bad, true); ok {
			t.Errorf("ParseTUN(%q) = ok, want false", bad)
		}
	}
	if got := FormatTUN(101.1, 0, true); got != "10110" {
		t.Errorf("FormatTUN FM = %q, want 10110", got)
	}
	if got := FormatTUN(0, 810, false); got != "00810" {
		t.Errorf("FormatTUN AM = %q, want 00810", got)
	}
}

func TestApplyLivePowerMuteVolInput(t *testing.T) {
	a := New(true, Defaults{})
	a.ApplyLive("PWR", "01")
	if !a.Power {
		t.Error("PWR 01 should power on")
	}
	a.ApplyLive("PWR", "00")
	if a.Power {
		t.Error("PWR 00 should power off")
	}
	a.ApplyLive("AMT", "01")
	if !a.Mute {
		t.Error("AMT 01 should mute")
	}
	a.ApplyLive("MVL", "50")
	if a.VolDisplay != 40 {
		t.Errorf("MVL 50 → %v, want 40", a.VolDisplay)
	}
	a.ApplyLive("SLI", "24")
	if a.InputID != "24" {
		t.Errorf("SLI 24 → %q", a.InputID)
	}
}

func TestApplyLiveTPD(t *testing.T) {
	a := New(true, Defaults{})
	a.ApplyLive("TPD", "F 82C 28")
	if a.TempC != 28 {
		t.Errorf("TempC = %d, want 28 (Celsius field of TPD)", a.TempC)
	}
}

func TestApplyLiveTunerFM(t *testing.T) {
	a := New(true, Defaults{})
	a.ApplyLive("SLI", "24") // FM input
	a.ApplyLive("TUN", "10110")
	if math.Abs(a.FmHz-101.1) > 1e-9 {
		t.Errorf("FmHz = %v, want 101.1", a.FmHz)
	}
	// Out-of-band values clamp.
	a.ApplyLive("TUN", "20000")
	if a.FmHz != 108 {
		t.Errorf("FmHz = %v, want clamped 108", a.FmHz)
	}
}

func TestApplyLiveCTL(t *testing.T) {
	a := New(true, Defaults{})
	a.ApplyLive("CTL", "+14")
	if a.CTL != 10 {
		t.Errorf("CTL = %v, want 10.0 dB", a.CTL)
	}
	a.ApplyLive("CTL", "QSTN") // must be ignored
	if a.CTL != 10 {
		t.Errorf("CTL changed by invalid frame: %v", a.CTL)
	}
}

func TestSnapshotVolumeAndTuner(t *testing.T) {
	a := New(true, Defaults{})
	a.SetVolDisplay(35.5)
	a.ApplyLive("SLI", "24")
	a.ApplyLive("TUN", "09850")
	s := a.Snapshot()
	if !s.Power {
		t.Error("fresh AVR should default powered on")
	}
	if s.VolDisplay != 35.5 {
		t.Errorf("snapshot VolDisplay = %v", s.VolDisplay)
	}
	if !s.IsFM || !s.IsTuner {
		t.Errorf("IsFM/IsTuner = %v/%v with input 24", s.IsFM, s.IsTuner)
	}
	if s.FreqStr != "98.50" {
		t.Errorf("FreqStr = %q, want 98.50", s.FreqStr)
	}
}

func TestSetInputClampsAndClearsNet(t *testing.T) {
	a := New(true, Defaults{})
	a.SetMockNetTopMenu()
	if !a.Snapshot().IsNet {
		t.Fatal("expected NET input after mock menu")
	}
	a.SetInput("11")
	s := a.Snapshot()
	if s.IsNet {
		t.Error("switching away from NET should clear net state")
	}
	// SetInput does not validate ids (caller-side concern); net state is
	// still cleared for anything that isn't NET.
	a.SetInput("ZZ")
	if a.Snapshot().InputID != "ZZ" || a.Snapshot().IsNet {
		t.Errorf("unexpected state after SetInput(ZZ): %+v", a.Snapshot().InputID)
	}
}

func TestVolClampHalfSteps(t *testing.T) {
	a := New(true, Defaults{})
	a.SetVolDisplay(37.7)
	if got := a.Snapshot().VolDisplay; got != 37.5 {
		t.Errorf("vol = %v, want snapped to 37.5", got)
	}
	a.SetVolDisplay(-3)
	if got := a.Snapshot().VolDisplay; got != 0 {
		t.Errorf("vol = %v, want clamped 0", got)
	}
	a.SetVolDisplay(150)
	if got := a.Snapshot().VolDisplay; got != 100 {
		t.Errorf("vol = %v, want clamped 100", got)
	}
}
