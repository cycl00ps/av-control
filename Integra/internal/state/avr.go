package state

import (
	"fmt"
	"regexp"
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

// tpdCelsius matches eISCP TPD like "F 82C 28" (Fahrenheit then Celsius).
var tpdCelsius = regexp.MustCompile(`(?i)F\s*(\d+)\s*C\s*(\d+)`)

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
	CTL        float64 // centre trim dB (−12.0…+12.0, 0.5 steps)
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

	// NET / TuneIn
	NetServiceID   string
	NetServiceName string
	NetListTitle   string
	NetList        []NetListItem
	NetListCache   []NetListItem // accumulated rows at current menu layer (More ↓)
	NetBrowsePath  []int         // NLSL steps from service root
	NetUIBrowse    bool          // true = show list/search (even if audio still playing)
	NetNST         string        // raw NST
	NetTitle       string        // NTI
	NetArtist      string        // NAT
	NetAlbum       string        // NAL
	NetTime        string        // NTM
	NetFormat      string        // NFI

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
		CTL:        ParseCTLEnv(d.CenterLevel),
		LTN:        d.LateNight,
		DSS:        "00",
		EQS:        "00",
		FmHz:       92.1,
		AmKhz:      810,
		TempC:      28,
		TPDRaw:     "F 82C 28",
		IFA:        "HDMI 4,DTS-HD,48 kHz,5.1 ch,Straight Decode,3.1 ch,",
		IFV:        "HDMI 4,4K(3840x2160) 59 Hz,YCbCr422,24bit,MAIN,4K(3840x2160) 59 Hz,YCbCr422,24bit,,,",
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

// ParseCTLEnv parses human defaults like "+10" / "+10.0" / "-1.5" as dB.
func ParseCTLEnv(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return clampCTL(n)
}

// CTLWireToDb parses an eISCP CTL parameter. Wire units are signed hex
// half-dB steps: "+14" = +10.0 dB, "+0A" = +5.0 dB, "-01" = −0.5 dB.
func CTLWireToDb(param string) (float64, bool) {
	p := strings.TrimSpace(param)
	if p == "" || strings.EqualFold(p, "QSTN") || strings.EqualFold(p, "UP") || strings.EqualFold(p, "DOWN") || strings.EqualFold(p, "N/A") {
		return 0, false
	}
	if p == "000" || p == "00" {
		return 0, true
	}
	sign := 1.0
	if strings.HasPrefix(p, "+") {
		p = p[1:]
	} else if strings.HasPrefix(p, "-") {
		sign = -1
		p = p[1:]
	}
	v, err := strconv.ParseInt(p, 16, 64)
	if err != nil {
		return 0, false
	}
	return clampCTL(sign * float64(v) / 2), true
}

// FormatCTLParam encodes dB as eISCP CTL wire (hex half-steps).
func FormatCTLParam(db float64) string {
	n := int(clampCTL(db) * 2)
	if n == 0 {
		return "000"
	}
	if n > 0 {
		return fmt.Sprintf("+%02X", n)
	}
	return fmt.Sprintf("-%02X", -n)
}

func formatCTLStr(db float64) string {
	db = float64(int(db*2)) / 2
	if db > 0 {
		return fmt.Sprintf("+%.1f dB", db)
	}
	if db < 0 {
		return fmt.Sprintf("%.1f dB", db)
	}
	return "0.0 dB"
}

func splitCSV(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		out = append(out, p)
	}
	return out
}

func csvField(parts []string, i int) string {
	if i < 0 || i >= len(parts) {
		return ""
	}
	return parts[i]
}

func joinAV(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// ParseIFA splits eISCP IFA: jack,codec,rate,in-ch,mode,out-ch,...
func ParseIFA(s string) (in, out, hint string) {
	parts := splitCSV(s)
	inCh := csvField(parts, 3)
	outCh := csvField(parts, 5)
	in = joinAV(csvField(parts, 1), csvField(parts, 2), inCh)
	out = joinAV(csvField(parts, 4), outCh)
	if in == "" && out == "" {
		return s, "", ""
	}
	if inCh != "" && outCh != "" && !strings.EqualFold(inCh, outCh) {
		hint = "Processed"
	} else if in != "" && out != "" {
		hint = "Pass-through"
	}
	return in, out, hint
}

// ParseIFV splits eISCP IFV: jack,in-res,in-color,in-bit,hdmi-out,out-res,out-color,out-bit,...
func ParseIFV(s string) (in, out, hint string) {
	parts := splitCSV(s)
	inRes, inColor, inBit := csvField(parts, 1), csvField(parts, 2), csvField(parts, 3)
	outRes, outColor, outBit := csvField(parts, 5), csvField(parts, 6), csvField(parts, 7)
	in = joinAV(inRes, inColor, inBit)
	out = joinAV(csvField(parts, 4), outRes, outColor, outBit)
	if in == "" && out == "" {
		return s, "", ""
	}
	if (inRes != "" || outRes != "") &&
		(!strings.EqualFold(inRes, outRes) || !strings.EqualFold(inColor, outColor) || !strings.EqualFold(inBit, outBit)) {
		hint = "Processed"
	} else if in != "" && out != "" {
		hint = "Pass-through"
	}
	return in, out, hint
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
	CTL        float64
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
	AudioIn    string
	AudioOut   string
	AudioHint  string
	VideoIn    string
	VideoOut   string
	VideoHint  string
	IsTuner    bool
	IsFM       bool
	FreqStr    string
	FreqUnit   string
	Preset     int
	IsNet      bool
	IsTuneIn   bool
	NetPlaying bool
	NetServiceID   string
	NetServiceName string
	NetListTitle   string
	NetList        []NetListItem
	NetListCache   []NetListItem
	NetBrowsePath  []int
	NetUIBrowse    bool
	NetCanPin      bool
	NetNST         string
	NetPlayLabel   string
	NetTitle       string
	NetArtist      string
	NetAlbum       string
	NetTime        string
	NetFormat      string
	NetHint        string
	NetServices    []NetService
	Mock           bool
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
	case "00":
		lmdName = "Stereo"
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
	isNet := a.InputID == "2B"
	freq := ""
	unit := ""
	fld := name
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
	if isNet && a.NetTitle != "" {
		fld = a.NetTitle
	} else if isNet && a.NetListTitle != "" {
		fld = a.NetListTitle
	} else if isNet && a.NetServiceName != "" {
		fld = a.NetServiceName
	}
	ctlStr := formatCTLStr(a.CTL)
	audioIn, audioOut, audioHint := ParseIFA(a.IFA)
	videoIn, videoOut, videoHint := ParseIFV(a.IFV)
	netList := sortNetList(a.NetList)
	netCache := sortNetList(a.NetListCache)
	if len(netCache) == 0 {
		netCache = netList
	} else {
		// Prefer amp line numbers from the current page when texts match.
		for i := range netCache {
			for _, cur := range netList {
				if strings.EqualFold(cur.Text, netCache[i].Text) {
					netCache[i].Line = cur.Line
					break
				}
			}
		}
	}
	path := append([]int(nil), a.NetBrowsePath...)
	svcID := a.NetServiceID
	canPin := strings.EqualFold(svcID, "0e") && len(path) > 0 && (IsNetPlaying(a.NetNST) || a.NetTitle != "")
	return Snapshot{
		Power: a.Power, Mute: a.Mute, VolDisplay: a.VolDisplay, VolStr: formatVol(a.VolDisplay),
		InputID: a.InputID, InputName: name, LMD: a.LMD, LMDName: lmdName,
		CTL: a.CTL, CTLStr: ctlStr, LTN: a.LTN, DSS: a.DSS, DSSName: dssName, EQS: a.EQS,
		Fld: fld, TempC: a.TempC, TPDRaw: a.TPDRaw, IFA: a.IFA, IFV: a.IFV,
		AudioIn: audioIn, AudioOut: audioOut, AudioHint: audioHint,
		VideoIn: videoIn, VideoOut: videoOut, VideoHint: videoHint,
		IsTuner: isTuner, IsFM: isFM, FreqStr: freq, FreqUnit: unit, Preset: a.Preset,
		IsNet: isNet, IsTuneIn: strings.EqualFold(svcID, "0e"),
		NetPlaying: IsNetPlaying(a.NetNST),
		NetServiceID: svcID, NetServiceName: a.NetServiceName,
		NetListTitle: a.NetListTitle, NetList: netList, NetListCache: netCache,
		NetBrowsePath: path, NetUIBrowse: a.NetUIBrowse, NetCanPin: canPin,
		NetNST: a.NetNST, NetPlayLabel: NetPlayStateLabel(a.NetNST),
		NetTitle: a.NetTitle, NetArtist: a.NetArtist, NetAlbum: a.NetAlbum,
		NetTime: a.NetTime, NetFormat: a.NetFormat,
		NetHint: NetServiceHint(svcID), NetServices: FallbackNetServices,
		Mock: a.Mock,
	}
}

func sortNetList(in []NetListItem) []NetListItem {
	out := append([]NetListItem(nil), in...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Line < out[i].Line || (out[j].Line == out[i].Line && out[j].Text < out[i].Text) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
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
	a.CTL = ParseCTLEnv(a.Defaults.CenterLevel)
	a.LTN = a.Defaults.LateNight
	if a.LTN == "" {
		a.LTN = "01"
	}
	if a.Mock {
		a.refreshIFALocked()
	}
}

func (a *AVR) refreshIFALocked() {
	if a.LMD == "82" {
		a.IFA = "HDMI 4,DTS-HD,48 kHz,5.1 ch,DTS Neural:X,3.1 ch,"
	} else {
		a.IFA = "HDMI 4,DTS-HD,48 kHz,5.1 ch,Straight Decode,3.1 ch,"
	}
}

// Mutators used by mock (and after live confirms)

func (a *AVR) SetPower(on bool)          { a.mu.Lock(); a.Power = on; a.mu.Unlock() }
func (a *AVR) SetMute(m bool)            { a.mu.Lock(); a.Mute = m; a.mu.Unlock() }
func (a *AVR) ToggleMute() bool          { a.mu.Lock(); defer a.mu.Unlock(); a.Mute = !a.Mute; return a.Mute }
func (a *AVR) SetVolDisplay(v float64)   { a.mu.Lock(); a.VolDisplay = clampVol(v); a.mu.Unlock() }
func (a *AVR) NudgeVol(delta float64)    { a.mu.Lock(); a.VolDisplay = clampVol(a.VolDisplay + delta); a.mu.Unlock() }
func (a *AVR) SetInput(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.InputID = id
	a.Preset = 0
	if id != "2B" {
		a.clearNetLocked()
	}
}

func (a *AVR) clearNetLocked() {
	a.NetServiceID = ""
	a.NetServiceName = ""
	a.NetListTitle = ""
	a.NetList = nil
	a.NetListCache = nil
	a.NetBrowsePath = nil
	a.NetUIBrowse = false
	a.NetNST = ""
	a.NetTitle = ""
	a.NetArtist = ""
	a.NetAlbum = ""
	a.NetTime = ""
	a.NetFormat = ""
}

func (a *AVR) SetNetService(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.NetServiceID = strings.ToLower(strings.TrimSpace(id))
	a.NetServiceName = NetServiceName(a.NetServiceID)
	a.NetListTitle = ""
	a.NetList = nil
	a.NetListCache = nil
	a.NetBrowsePath = nil
	a.NetUIBrowse = true
	a.NetTitle = ""
	a.NetArtist = ""
	a.NetAlbum = ""
	a.NetTime = ""
	a.NetFormat = ""
	a.NetNST = ""
}

// SetNetUIBrowse toggles list vs now-playing panel (independent of amp NST/NTI).
func (a *AVR) SetNetUIBrowse(browse bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.NetUIBrowse = browse
}

func (a *AVR) NetUIBrowseMode() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.NetUIBrowse
}

// PushNetBrowse appends a menu line to the TuneIn path stack.
func (a *AVR) PushNetBrowse(line int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if line < 0 || line > 9 {
		return
	}
	a.NetBrowsePath = append(a.NetBrowsePath, line)
}

// PopNetBrowse removes the last browse step (Back).
func (a *AVR) PopNetBrowse() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n := len(a.NetBrowsePath); n > 0 {
		a.NetBrowsePath = a.NetBrowsePath[:n-1]
	}
}

// ResetNetBrowse clears the path (service root).
func (a *AVR) ResetNetBrowse() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.NetBrowsePath = nil
}

// SetNetBrowsePath replaces the path (e.g. after pin play).
func (a *AVR) SetNetBrowsePath(steps []int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.NetBrowsePath = append([]int(nil), steps...)
}

func (a *AVR) NetBrowsePathCopy() []int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]int(nil), a.NetBrowsePath...)
}

func mergeNetCache(cache []NetListItem, item NetListItem) []NetListItem {
	for i := range cache {
		if strings.EqualFold(cache[i].Text, item.Text) {
			cache[i] = item
			return cache
		}
	}
	return append(cache, item)
}

func (a *AVR) SetMockNetTopMenu() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.InputID = "2B"
	a.NetServiceID = "0e"
	a.NetServiceName = "TuneIn Radio"
	a.NetListTitle = "TuneIn Radio"
	a.NetList = []NetListItem{
		{0, "My Presets"}, {1, "Local Radio"}, {2, "Music"}, {3, "Talk"},
		{4, "Sports"}, {5, "By Location"}, {6, "By Language"}, {7, "Podcasts"}, {8, "Login"},
	}
	a.NetListCache = append([]NetListItem(nil), a.NetList...)
	a.NetBrowsePath = nil
	a.NetUIBrowse = true
	a.NetNST = "Sxx1"
}

func (a *AVR) MockNetSelect(line int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if line < 0 || line > 9 {
		return
	}
	var text string
	for _, it := range a.NetList {
		if it.Line == line {
			text = it.Text
			break
		}
	}
	if text == "" && line < len(a.NetList) {
		text = a.NetList[line].Text
	}
	a.NetBrowsePath = append(a.NetBrowsePath, line)
	if text == "Local Radio" || text == "Stations" || (strings.Contains(strings.ToLower(text), "radio") && !strings.Contains(text, "(")) {
		a.NetListTitle = text
		a.NetList = []NetListItem{
			{0, "Stations"},
			{1, "2CR Chinese Radio Station (Chinese Talk)"},
			{2, "2MBS Fine Music Sydney 102.5 (Classical)"},
			{3, "2MCR 100.3 (Music)"},
		}
		a.NetListCache = append([]NetListItem(nil), a.NetList...)
		a.NetUIBrowse = true
		a.NetNST = "Sxx1"
		a.NetTitle = ""
		return
	}
	a.NetTitle = text
	a.NetListTitle = ""
	a.NetList = nil
	a.NetListCache = nil
	a.NetUIBrowse = false
	a.NetNST = "Pxx1"
	a.NetFormat = "AAC/256kbps/"
	a.NetTime = "00:00:01/--:--:--"
}

func (a *AVR) MockNetBack() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n := len(a.NetBrowsePath); n > 0 {
		a.NetBrowsePath = a.NetBrowsePath[:n-1]
	}
	a.NetUIBrowse = true
	if a.NetTitle != "" {
		a.NetTitle = ""
		a.NetNST = "Sxx1"
		a.NetFormat = ""
		a.NetTime = ""
		a.NetListTitle = "Local Radio"
		a.NetList = []NetListItem{
			{0, "Stations"},
			{1, "2CR Chinese Radio Station (Chinese Talk)"},
			{2, "2MBS Fine Music Sydney 102.5 (Classical)"},
			{3, "2MCR 100.3 (Music)"},
		}
		a.NetListCache = append([]NetListItem(nil), a.NetList...)
		return
	}
	a.SetMockNetTopMenuUnlocked()
}

func (a *AVR) SetMockNetTopMenuUnlocked() {
	a.NetServiceID = "0e"
	a.NetServiceName = "TuneIn Radio"
	a.NetListTitle = "TuneIn Radio"
	a.NetList = []NetListItem{
		{0, "My Presets"}, {1, "Local Radio"}, {2, "Music"}, {3, "Talk"},
		{4, "Sports"}, {5, "By Location"}, {6, "By Language"}, {7, "Podcasts"}, {8, "Login"},
	}
	a.NetListCache = append([]NetListItem(nil), a.NetList...)
	a.NetUIBrowse = true
	a.NetNST = "Sxx1"
	a.NetTitle = ""
}
func (a *AVR) SetLMD(v string) {
	a.mu.Lock()
	a.LMD = v
	if a.Mock {
		a.refreshIFALocked()
	}
	a.mu.Unlock()
}
func (a *AVR) SetCTL(n float64)           { a.mu.Lock(); a.CTL = clampCTL(n); a.mu.Unlock() }
func (a *AVR) NudgeCTL(d float64)         { a.mu.Lock(); a.CTL = clampCTL(a.CTL + d); a.mu.Unlock() }
func (a *AVR) SetLTN(v string)           { a.mu.Lock(); a.LTN = v; a.mu.Unlock() }
func (a *AVR) SetDSS(v string)           { a.mu.Lock(); a.DSS = v; a.mu.Unlock() }
func (a *AVR) SetEQS(v string)           { a.mu.Lock(); a.EQS = v; a.mu.Unlock() }

func (a *AVR) TuneNudge(up bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Preset = 0
	if a.InputID == "24" {
		d := -0.1
		if up {
			d = 0.1
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

// NextTUNParam steps the local frequency and returns the absolute eISCP TUN wire value.
func (a *AVR) NextTUNParam(up bool) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Preset = 0
	switch a.InputID {
	case "24":
		d := -0.1
		if up {
			d = 0.1
		}
		a.FmHz = clampFm(a.FmHz + d)
		return FormatTUN(a.FmHz, 0, true), true
	case "25":
		d := -9
		if up {
			d = 9
		}
		a.AmKhz = clampAm(a.AmKhz + d)
		return FormatTUN(0, a.AmKhz, false), true
	default:
		return "", false
	}
}

// CurrentTUNParam returns the absolute eISCP TUN wire for the current frequency.
func (a *AVR) CurrentTUNParam() (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	switch a.InputID {
	case "24":
		return FormatTUN(a.FmHz, 0, true), true
	case "25":
		return FormatTUN(0, a.AmKhz, false), true
	default:
		return "", false
	}
}

// SnapFMGrid rounds FM to the nearest 0.1 MHz (AU channel spacing).
func (a *AVR) SnapFMGrid() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.InputID != "24" {
		return
	}
	snapped := clampFm(float64(int(a.FmHz*10+0.5)) / 10)
	if snapped != a.FmHz {
		a.Preset = 0
	}
	a.FmHz = snapped
}

func (a *AVR) SetPreset(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n < 1 || n > 40 {
		return
	}
	a.Preset = n
	// Mock only: seed a plausible frequency for the slot.
	if !a.Mock {
		return
	}
	if a.InputID == "24" {
		a.FmHz = clampFm(88.1 + float64(n-1)*0.4)
	} else if a.InputID == "25" {
		a.AmKhz = clampAm(530 + (n-1)*27)
	}
}

func (a *AVR) ClearPreset() {
	a.mu.Lock()
	a.Preset = 0
	a.mu.Unlock()
}

// ParseTUN decodes eISCP TUN frequency. FM: 5 digits = MHz*100; AM: 5 digits = kHz.
func ParseTUN(param string, preferFM bool) (fmHz float64, amKhz int, ok bool) {
	p := strings.TrimSpace(param)
	if p == "" || strings.EqualFold(p, "N/A") || strings.EqualFold(p, "QSTN") ||
		strings.EqualFold(p, "UP") || strings.EqualFold(p, "DOWN") {
		return 0, 0, false
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 0, 0, false
	}
	// FM band encodings are >= 8750 (87.50 MHz); AM is <= 1710.
	if preferFM || n >= 8750 {
		return float64(n) / 100.0, 0, true
	}
	return 0, n, true
}

// FormatTUN encodes display frequency to eISCP TUN wire (5 digits).
func FormatTUN(fmHz float64, amKhz int, isFM bool) string {
	if isFM {
		n := int(fmHz*100 + 0.5)
		if n < 0 {
			n = 0
		}
		return fmt.Sprintf("%05d", n)
	}
	if amKhz < 0 {
		amKhz = 0
	}
	return fmt.Sprintf("%05d", amKhz)
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
		id := strings.ToUpper(param)
		if a.InputID != id {
			a.Preset = 0
			if id != "2B" {
				a.clearNetLocked()
			}
		}
		a.InputID = id
	case "LMD":
		a.LMD = param
		if a.Mock {
			a.refreshIFALocked()
		}
	case "CTL":
		if db, ok := CTLWireToDb(param); ok {
			a.CTL = db
		}
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
		// Wire shape: "F 82C 28" = 82 °F and 28 °C — use the value after C.
		if m := tpdCelsius.FindStringSubmatch(param); len(m) == 3 {
			if n, err := strconv.Atoi(m[2]); err == nil {
				a.TempC = n
			}
		}
	case "IFA":
		a.IFA = param
	case "IFV":
		a.IFV = param
	case "TUN":
		isFM := a.InputID == "24"
		fm, am, ok := ParseTUN(param, isFM)
		if !ok {
			break
		}
		if isFM {
			a.FmHz = clampFm(fm)
		} else {
			a.AmKhz = clampAm(am)
		}
	case "PRS":
		p := strings.TrimSpace(param)
		if p == "" || p == "00" || strings.EqualFold(p, "N/A") || strings.EqualFold(p, "QSTN") {
			a.Preset = 0
			break
		}
		if n, err := strconv.ParseInt(p, 16, 64); err == nil && n >= 1 && n <= 40 {
			a.Preset = int(n)
		}
	case "NSV":
		p := strings.ToLower(strings.TrimSpace(param))
		if len(p) >= 2 && !strings.EqualFold(p, "N/A") && !strings.EqualFold(p, "QSTN") {
			a.NetServiceID = p[:2]
			a.NetServiceName = NetServiceName(a.NetServiceID)
		}
	case "NLT":
		title := ParseNLTTitle(param)
		if title != "" && !strings.EqualFold(title, a.NetListTitle) {
			a.NetListCache = nil
		}
		a.NetListTitle = title
		a.NetList = nil
	case "NLS":
		p := strings.TrimSpace(param)
		if strings.EqualFold(p, "N/A") {
			break
		}
		if item, ok := ParseNLSItem(p); ok {
			replaced := false
			for i := range a.NetList {
				if a.NetList[i].Line == item.Line {
					a.NetList[i] = item
					replaced = true
					break
				}
			}
			if !replaced {
				a.NetList = append(a.NetList, item)
			}
			a.NetListCache = mergeNetCache(a.NetListCache, item)
		}
	case "NST":
		if param != "" && !strings.EqualFold(param, "N/A") {
			a.NetNST = param
		}
	case "NTI":
		t := strings.TrimSpace(param)
		if t != "" {
			a.NetTitle = t
		}
	case "NAT":
		t := strings.TrimSpace(param)
		if t != "" {
			a.NetArtist = t
		}
	case "NAL":
		t := strings.TrimSpace(param)
		if t != "" {
			a.NetAlbum = t
		}
	case "NTM":
		if param != "" && !strings.EqualFold(param, "N/A") {
			a.NetTime = param
		}
	case "NFI":
		t := strings.TrimSpace(param)
		if t != "" {
			a.NetFormat = t
		}
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

func clampCTL(n float64) float64 {
	n = float64(int(n*2)) / 2
	if n < -12 {
		return -12
	}
	if n > 12 {
		return 12
	}
	return n
}

func clampFm(hz float64) float64 {
	// AU FM channel spacing is 0.1 MHz.
	hz = float64(int(hz*10+0.5)) / 10
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
