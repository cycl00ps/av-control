package state

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type Input struct {
	ID   string
	Name string
}

var Inputs = []Input{
	{"10", "XBOX"}, {"02", "GAME"}, {"01", "PS5"}, {"11", "Shield"},
	{"05", "PC"}, {"03", "AUX"}, {"23", "CD"}, {"12", "TV"},
	{"22", "PHONO"}, {"25", "AM"}, {"24", "FM"}, {"2B", "NET"}, {"2E", "BT"},
}

type Defaults struct {
	ListeningMode string // "40"
	MasterVolume  string // hex "50"
	CenterLevel   string // "+10"
	LateNight     string // "01"
}

type AVR struct {
	mu sync.RWMutex

	Power      bool
	Mute       bool
	VolDisplay float64 // panel units
	InputID    string
	LMD        string
	CTL        int
	LTN        string
	DSS        string
	EQS        string
	FmHz       float64
	AmKhz      int
	Preset     int // 0 = none
	TempC      int
	TPDRaw     string
	IFA        string
	IFV        string

	Defaults Defaults
	Mock     bool
}

func New(mock bool, d Defaults) *AVR {
	a := &AVR{
		Power:      true,
		Mute:       false,
		VolDisplay: HexVolToDisplay(d.MasterVolume),
		InputID:    "11",
		LMD:        d.ListeningMode,
		CTL:        ParseCTL(d.CenterLevel),
		LTN:        d.LateNight,
		DSS:        "00",
		EQS:        "00",
		FmHz:       92.1,
		AmKhz:      810,
		TempC:      82,
		TPDRaw:     "F 82C 28",
		IFA:        "HDMI 4 · DTS-HD · 48 kHz · 5.1 → 3.1 (Straight)",
		IFV:        "4K 3840×2160 @ 59 Hz · YCbCr 4:2:2 · 24-bit",
		Defaults:   d,
		Mock:       mock,
	}
	if a.LMD == "" {
		a.LMD = "40"
	}
	if a.LTN == "" {
		a.LTN = "01"
	}
	return a
}

func HexVolToDisplay(hex string) float64 {
	v, err := strconv.ParseInt(strings.TrimSpace(hex), 16, 64)
	if err != nil {
		return 40
	}
	return float64(v) / 2
}

func DisplayToHexVol(d float64) string {
	n := int(d * 2)
	if n < 0 {
		n = 0
	}
	if n > 200 {
		n = 200
	}
	return fmt.Sprintf("%02X", n)
}

func ParseCTL(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "+")
	n, _ := strconv.Atoi(s)
	if n < -12 {
		n = -12
	}
	if n > 12 {
		n = 12
	}
	return n
}

func FormatCTL(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%02d", n)
	}
	if n < 0 {
		return fmt.Sprintf("%03d", n) // -01 style often used; eISCP uses +10 / 000 / -01
	}
	return "000"
}

func FormatCTLParam(n int) string {
	if n == 0 {
		return "000"
	}
	if n > 0 {
		return fmt.Sprintf("+%02d", n)
	}
	return fmt.Sprintf("-%02d", -n)
}

type Snapshot struct {
	Power      bool
	Mute       bool
	VolDisplay float64
	VolStr     string
	InputID    string
	InputName  string
	LMD        string
	LMDName    string
	CTL        int
	CTLStr     string
	LTN        string
	DSS        string
	DSSName    string
	EQS        string
	Fld        string
	TempC      int
	TPDRaw     string
	IFA        string
	IFV        string
	IsTuner    bool
	IsFM       bool
	FreqStr    string
	FreqUnit   string
	Preset     int
	Mock       bool
}

func (a *AVR) Snapshot() Snapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	name := a.InputID
	for _, in := range Inputs {
		if in.ID == a.InputID {
			name = in.Name
			break
		}
	}
	lmdName := a.LMD
	switch a.LMD {
	case "40":
		lmdName = "Straight Decode"
	case "82":
		lmdName = "Neural:X"
	}
	dssName := "Dirac Off"
	switch a.DSS {
	case "01":
		dssName = "Dirac Slot 1"
	case "02":
		dssName = "Dirac Slot 2"
	case "03":
		dssName = "Dirac Slot 3"
	}
	isFM := a.InputID == "24"
	isAM := a.InputID == "25"
	isTuner := isFM || isAM
	freq := ""
	unit := ""
	fld := fmt.Sprintf("%-10s%s", name, formatVol(a.VolDisplay))
	if isTuner {
		if isFM {
			freq = fmt.Sprintf("%.2f", a.FmHz)
			unit = "MHz"
			fld = fmt.Sprintf("FM  %s MHz", freq)
		} else {
			freq = fmt.Sprintf("%d", a.AmKhz)
			unit = "kHz"
			fld = fmt.Sprintf("AM  %s kHz", freq)
		}
	}
	ctlStr := "0 dB"
	if a.CTL > 0 {
		ctlStr = fmt.Sprintf("+%d dB", a.CTL)
	} else if a.CTL < 0 {
		ctlStr = fmt.Sprintf("%d dB", a.CTL)
	}
	return Snapshot{
		Power: a.Power, Mute: a.Mute, VolDisplay: a.VolDisplay, VolStr: formatVol(a.VolDisplay),
		InputID: a.InputID, InputName: name, LMD: a.LMD, LMDName: lmdName,
		CTL: a.CTL, CTLStr: ctlStr, LTN: a.LTN, DSS: a.DSS, DSSName: dssName, EQS: a.EQS,
		Fld: fld, TempC: a.TempC, TPDRaw: a.TPDRaw, IFA: a.IFA, IFV: a.IFV,
		IsTuner: isTuner, IsFM: isFM, FreqStr: freq, FreqUnit: unit, Preset: a.Preset, Mock: a.Mock,
	}
}

func formatVol(v float64) string {
	return fmt.Sprintf("%.1f", float64(int(v*2))/2)
}

func (a *AVR) ApplyDefaults() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.LMD = a.Defaults.ListeningMode
	if a.LMD == "" {
		a.LMD = "40"
	}
	a.VolDisplay = HexVolToDisplay(a.Defaults.MasterVolume)
	a.CTL = ParseCTL(a.Defaults.CenterLevel)
	a.LTN = a.Defaults.LateNight
	if a.LTN == "" {
		a.LTN = "01"
	}
	a.refreshIFALocked()
}

func (a *AVR) refreshIFALocked() {
	if a.LMD == "82" {
		a.IFA = "HDMI 4 · DTS-HD · 48 kHz · 5.1 · Neural:X → 3.1"
	} else {
		a.IFA = "HDMI 4 · DTS-HD · 48 kHz · 5.1 → 3.1 (Straight)"
	}
}

// Mutators used by mock (and after live confirms)

func (a *AVR) SetPower(on bool)          { a.mu.Lock(); a.Power = on; a.mu.Unlock() }
func (a *AVR) SetMute(m bool)            { a.mu.Lock(); a.Mute = m; a.mu.Unlock() }
func (a *AVR) ToggleMute() bool          { a.mu.Lock(); defer a.mu.Unlock(); a.Mute = !a.Mute; return a.Mute }
func (a *AVR) SetVolDisplay(v float64)   { a.mu.Lock(); a.VolDisplay = clampVol(v); a.mu.Unlock() }
func (a *AVR) NudgeVol(delta float64)    { a.mu.Lock(); a.VolDisplay = clampVol(a.VolDisplay + delta); a.mu.Unlock() }
func (a *AVR) SetInput(id string)        { a.mu.Lock(); a.InputID = id; a.Preset = 0; a.mu.Unlock() }
func (a *AVR) SetLMD(v string)           { a.mu.Lock(); a.LMD = v; a.refreshIFALocked(); a.mu.Unlock() }
func (a *AVR) SetCTL(n int)              { a.mu.Lock(); a.CTL = clampCTL(n); a.mu.Unlock() }
func (a *AVR) NudgeCTL(d int)            { a.mu.Lock(); a.CTL = clampCTL(a.CTL + d); a.mu.Unlock() }
func (a *AVR) SetLTN(v string)           { a.mu.Lock(); a.LTN = v; a.mu.Unlock() }
func (a *AVR) SetDSS(v string)           { a.mu.Lock(); a.DSS = v; a.mu.Unlock() }
func (a *AVR) SetEQS(v string)           { a.mu.Lock(); a.EQS = v; a.mu.Unlock() }

func (a *AVR) TuneNudge(up bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Preset = 0
	if a.InputID == "24" {
		d := -0.05
		if up {
			d = 0.05
		}
		a.FmHz = clampFm(a.FmHz + d)
	} else if a.InputID == "25" {
		d := -9
		if up {
			d = 9
		}
		a.AmKhz = clampAm(a.AmKhz + d)
	}
}

func (a *AVR) SetPreset(n int) {
	fm := []float64{88.5, 90.9, 92.1, 94.5, 96.9, 99.7, 101.7, 105.3}
	am := []int{540, 630, 720, 810, 900, 1010, 1170, 1440}
	a.mu.Lock()
	defer a.mu.Unlock()
	if n < 1 || n > 8 {
		return
	}
	a.Preset = n
	i := n - 1
	if a.InputID == "24" {
		a.FmHz = fm[i]
	} else if a.InputID == "25" {
		a.AmKhz = am[i]
	}
}

func (a *AVR) ApplyLive(cmd, param string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch cmd {
	case "PWR":
		a.Power = param == "01"
	case "AMT":
		a.Mute = param == "01"
	case "MVL":
		if v, err := strconv.ParseInt(param, 16, 64); err == nil {
			a.VolDisplay = float64(v) / 2
		}
	case "SLI":
		a.InputID = strings.ToUpper(param)
	case "LMD":
		a.LMD = param
		a.refreshIFALocked()
	case "CTL":
		a.CTL = ParseCTL(param)
	case "LTN":
		a.LTN = param
	case "DSS":
		if len(param) >= 3 {
			a.DSS = param[len(param)-2:]
		} else {
			a.DSS = param
		}
	case "EQS":
		a.EQS = param
	case "TPD":
		a.TPDRaw = param
		if i := strings.Index(strings.ToUpper(param), "C"); i > 0 {
			// find digits before C
			j := i - 1
			for j >= 0 && param[j] >= '0' && param[j] <= '9' {
				j--
			}
			if n, err := strconv.Atoi(param[j+1 : i]); err == nil {
				a.TempC = n
			}
		}
	case "IFA":
		a.IFA = param
	case "IFV":
		a.IFV = param
	}
}

func clampVol(v float64) float64 {
	v = float64(int(v*2)) / 2
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func clampCTL(n int) int {
	if n < -12 {
		return -12
	}
	if n > 12 {
		return 12
	}
	return n
}

func clampFm(hz float64) float64 {
	hz = float64(int(hz*20+0.5)) / 20
	if hz < 87.5 {
		return 87.5
	}
	if hz > 108 {
		return 108
	}
	return hz
}

func clampAm(k int) int {
	base := 522
	k = base + ((k-base+4)/9)*9
	if k < 522 {
		return 522
	}
	if k > 1611 {
		return 1611
	}
	return k
}
