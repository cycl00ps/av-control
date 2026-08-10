package web

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cycl00ps/av-control/Integra/internal/eiscp"
	"github.com/cycl00ps/av-control/Integra/internal/state"
)

//go:embed templates/*.html
var tmplFS embed.FS

type Server struct {
	AVR     *state.AVR
	Client  *eiscp.Client // nil if mock
	Tmpl    *template.Template
	Base    string
	VolWarn float64 // display units; 0 disables loudness gate
	Pins    *state.PinStore
}

type pageData struct {
	state.Snapshot
	Inputs     []state.Input
	Base       string  // e.g. "" or "/av" when behind a path proxy
	VolWarn    float64 // loudness warn threshold (0 = off)
	VolDefault float64 // apply-defaults master volume (display)
	TuneInPins []state.TuneInPin
}

func New(avr *state.AVR, client *eiscp.Client, base string, volWarn float64) (*Server, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if volWarn < 0 {
		volWarn = 0
	}
	t, err := template.New("").Funcs(template.FuncMap{
		"printf": fmt.Sprintf,
		"seq": func(a, b int) []int {
			out := make([]int, 0, b-a+1)
			for i := a; i <= b; i++ {
				out = append(out, i)
			}
			return out
		},
	}).ParseFS(tmplFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	pins := state.NewPinStore(defaultPinsPath())
	return &Server{AVR: avr, Client: client, Tmpl: t, Base: base, VolWarn: volWarn, Pins: pins}, nil
}

func defaultPinsPath() string {
	if d := strings.TrimSpace(os.Getenv("INTEGRA_DATA_DIR")); d != "" {
		return filepath.Join(d, "tunein-pins.json")
	}
	for _, d := range []string{"data", "Integra/data", "/opt/integra-web/data"} {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return filepath.Join(d, "tunein-pins.json")
		}
		if err := os.MkdirAll(d, 0o755); err == nil {
			return filepath.Join(d, "tunein-pins.json")
		}
	}
	return "data/tunein-pins.json"
}

func (s *Server) data() pageData {
	return pageData{
		Snapshot:   s.AVR.Snapshot(),
		Inputs:     state.Inputs,
		Base:       s.Base,
		VolWarn:    s.VolWarn,
		VolDefault: state.HexVolToDisplay(s.AVR.Defaults.MasterVolume),
		TuneInPins: s.Pins.List(),
	}
}

func (s *Server) Routes(mux *http.ServeMux, static http.Handler) {
	mux.Handle("/static/", http.StripPrefix("/static/", static))
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/partials/remote", s.handleRemote)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/cmd/", s.handleCmd)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.render(w, "index.html", s.data())
}

func (s *Server) handleRemote(w http.ResponseWriter, r *http.Request) {
	s.render(w, "remote.html", s.data())
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.render(w, "status.html", s.data())
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.Tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "template error", 500)
	}
}

func (s *Server) handleCmd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/cmd/")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	if err := s.apply(action, r); err != nil {
		log.Printf("cmd %s: %v", action, err)
	}
	s.render(w, "remote.html", s.data())
}

func (s *Server) apply(action string, r *http.Request) error {
	mock := s.Client == nil
	switch action {
	case "power":
		on := s.AVR.Snapshot().Power
		if on {
			if mock {
				s.AVR.SetPower(false)
				return nil
			}
			_, err := s.Client.Command("PWR", "00")
			if err == nil {
				s.AVR.SetPower(false)
			}
			return err
		}
		if mock {
			s.AVR.SetPower(true)
			return nil
		}
		_, err := s.Client.Command("PWR", "01")
		if err == nil {
			s.AVR.SetPower(true)
		}
		return err
	case "mute":
		if mock {
			s.AVR.ToggleMute()
			return nil
		}
		_, err := s.Client.Command("AMT", "TG")
		if err == nil {
			s.AVR.ToggleMute()
		}
		return err
	case "vol_up":
		return s.volDelta(r, 0.5)
	case "vol_down":
		return s.volDelta(r, -0.5)
	case "vol_set":
		v, _ := strconv.ParseFloat(r.FormValue("value"), 64)
		if err := s.requireLoudOK(r, s.AVR.Snapshot().VolDisplay, v); err != nil {
			return err
		}
		if mock {
			s.AVR.SetVolDisplay(v)
			return nil
		}
		return s.setVolDisplay(v)
	case "defaults":
		d := s.AVR.Defaults
		target := state.HexVolToDisplay(d.MasterVolume)
		if err := s.requireLoudOK(r, s.AVR.Snapshot().VolDisplay, target); err != nil {
			return err
		}
		s.AVR.ApplyDefaults()
		if mock {
			return nil
		}
		_, _ = s.Client.Command("LMD", d.ListeningMode)
		time.Sleep(60 * time.Millisecond)
		_, _ = s.Client.Command("MVL", d.MasterVolume)
		time.Sleep(60 * time.Millisecond)
		_, _ = s.Client.Command("CTL", state.FormatCTLParam(state.ParseCTLEnv(d.CenterLevel)))
		time.Sleep(60 * time.Millisecond)
		_, err := s.Client.Command("LTN", d.LateNight)
		return err
	case "input":
		id := r.FormValue("id")
		if mock {
			s.AVR.SetInput(id)
			return nil
		}
		_, err := s.Client.Command("SLI", id)
		if err != nil {
			return err
		}
		s.AVR.SetInput(id)
		if id == "24" || id == "25" {
			s.syncTuner()
		}
		if id == "2B" {
			// Leave service unselected until user picks; no auto-TuneIn.
			time.Sleep(150 * time.Millisecond)
			s.syncAV(false)
		} else {
			s.syncAV(true)
		}
		return nil
	case "lmd":
		v := r.FormValue("v")
		if mock {
			s.AVR.SetLMD(v)
			return nil
		}
		_, err := s.Client.Command("LMD", v)
		if err != nil {
			return err
		}
		s.AVR.SetLMD(v)
		s.syncAV(false)
		return nil
	case "ctl_set":
		n, _ := strconv.ParseFloat(r.FormValue("value"), 64)
		if mock {
			s.AVR.SetCTL(n)
			return nil
		}
		return s.setCTL(n)
	case "ctl_up":
		return s.ctlDelta(0.5)
	case "ctl_down":
		return s.ctlDelta(-0.5)
	case "ltn":
		v := r.FormValue("v")
		if mock {
			s.AVR.SetLTN(v)
			return nil
		}
		_, err := s.Client.Command("LTN", v)
		if err == nil {
			s.AVR.SetLTN(v)
		}
		return err
	case "dss":
		v := r.FormValue("v")
		if mock {
			s.AVR.SetDSS(v)
			return nil
		}
		_, err := s.Client.Command("DSS", "C"+v)
		if err == nil {
			s.AVR.SetDSS(v)
		}
		return err
	case "eqs":
		v := r.FormValue("v")
		if mock {
			s.AVR.SetEQS(v)
			return nil
		}
		_, err := s.Client.Command("EQS", v)
		if err == nil {
			s.AVR.SetEQS(v)
		}
		return err
	case "tune_up":
		if mock {
			s.AVR.TuneNudge(true)
			return nil
		}
		param, ok := s.AVR.NextTUNParam(true)
		if !ok {
			return fmt.Errorf("not on tuner")
		}
		resp, err := s.Client.Command("TUN", param)
		if err != nil {
			return err
		}
		s.applyTUNResp(resp)
		s.syncTunerFreq()
		return nil
	case "tune_down":
		if mock {
			s.AVR.TuneNudge(false)
			return nil
		}
		param, ok := s.AVR.NextTUNParam(false)
		if !ok {
			return fmt.Errorf("not on tuner")
		}
		resp, err := s.Client.Command("TUN", param)
		if err != nil {
			return err
		}
		s.applyTUNResp(resp)
		s.syncTunerFreq()
		return nil
	case "seek_up":
		if mock {
			s.AVR.TuneNudge(true)
			return nil
		}
		s.AVR.ClearPreset()
		_, err := s.Client.Command("TUN", "UP")
		return err
	case "seek_down":
		if mock {
			s.AVR.TuneNudge(false)
			return nil
		}
		s.AVR.ClearPreset()
		_, err := s.Client.Command("TUN", "DOWN")
		return err
	case "seek_stop":
		if mock {
			s.AVR.SnapFMGrid()
			return nil
		}
		// Lock the frequency the amp has reached so continuous seek stops,
		// snapped to the AU FM 0.1 MHz grid.
		s.syncTunerFreq()
		s.AVR.SnapFMGrid()
		param, ok := s.AVR.CurrentTUNParam()
		if !ok {
			return nil
		}
		resp, err := s.Client.Command("TUN", param)
		if err != nil {
			return err
		}
		s.applyTUNResp(resp)
		s.syncTunerFreq()
		return nil
	case "seek_tick":
		if mock {
			return nil
		}
		s.syncTunerFreq()
		return nil
	case "preset":
		n, _ := strconv.Atoi(r.FormValue("n"))
		if n < 1 || n > 40 {
			return fmt.Errorf("preset out of range")
		}
		if mock {
			s.AVR.SetPreset(n)
			return nil
		}
		resp, err := s.Client.Command("PRS", fmt.Sprintf("%02X", n))
		if err != nil {
			return err
		}
		s.AVR.ApplyLive("PRS", fmt.Sprintf("%02X", n))
		s.applyTUNResp(resp)
		s.syncTunerFreq()
		return nil
	case "preset_store":
		n, _ := strconv.Atoi(r.FormValue("n"))
		if n < 1 || n > 40 {
			return fmt.Errorf("preset out of range")
		}
		if mock {
			s.AVR.SetPreset(n)
			return nil
		}
		_, err := s.Client.Command("PRM", fmt.Sprintf("%02X", n))
		if err != nil {
			return err
		}
		s.AVR.ApplyLive("PRS", fmt.Sprintf("%02X", n))
		s.syncTunerFreq()
		return nil
	case "band":
		id := r.FormValue("id")
		if mock {
			s.AVR.SetInput(id)
			return nil
		}
		_, err := s.Client.Command("SLI", id)
		if err != nil {
			return err
		}
		s.AVR.SetInput(id)
		s.syncTuner()
		s.syncAV(true)
		return nil
	case "nsv":
		id := strings.ToLower(strings.TrimSpace(r.FormValue("id")))
		if id == "" {
			return fmt.Errorf("missing service id")
		}
		if mock {
			s.AVR.SetInput("2B")
			s.AVR.SetNetService(id)
			if strings.EqualFold(id, "0e") {
				s.AVR.SetMockNetTopMenu()
			}
			return nil
		}
		if s.AVR.Snapshot().InputID != "2B" {
			if _, err := s.Client.Command("SLI", "2B"); err != nil {
				return err
			}
			s.AVR.SetInput("2B")
			time.Sleep(100 * time.Millisecond)
		}
		s.AVR.SetNetService(id)
		s.AVR.ResetNetBrowse()
		msgs, err := s.Client.ExchangeAll("NSV"+state.NSVParam(id), 1500*time.Millisecond)
		if err != nil {
			return err
		}
		s.applyMsgs(msgs)
		if strings.EqualFold(id, "0e") && !msgsHaveNetList(msgs) {
			time.Sleep(120 * time.Millisecond)
			s.syncNetListFallback()
		} else if !msgsHaveNetList(msgs) {
			s.syncNetListFallback()
		}
		s.AVR.SetNetUIBrowse(true)
		return nil
	case "nls_select":
		line, err := strconv.Atoi(r.FormValue("line"))
		if err != nil || line < 0 || line > 9 {
			return fmt.Errorf("bad list line")
		}
		if mock {
			s.AVR.MockNetSelect(line)
			return nil
		}
		s.AVR.PushNetBrowse(line)
		msgs, err := s.Client.ExchangeAll(fmt.Sprintf("NLSL%d", line), 2500*time.Millisecond)
		if err != nil {
			s.AVR.PopNetBrowse()
			return err
		}
		s.applyMsgs(msgs)
		if msgsLookLikeNetPlayback(msgs) || msgsNetPlaying(msgs) {
			s.AVR.SetNetUIBrowse(false)
			s.syncNetMetaLight()
			return nil
		}
		if !msgsHaveNetList(msgs) {
			s.syncNetListFallback()
		}
		snap := s.AVR.Snapshot()
		if state.IsNetPlaying(snap.NetNST) && snap.NetTitle != "" && len(snap.NetList) == 0 {
			s.AVR.SetNetUIBrowse(false)
			return nil
		}
		s.AVR.SetNetUIBrowse(true)
		return nil
	case "net_back":
		if mock {
			s.AVR.MockNetBack()
			return nil
		}
		msgs, err := s.Client.ExchangeAll("NTCRETURN", 1500*time.Millisecond)
		if err != nil {
			return err
		}
		s.applyMsgs(msgs)
		s.AVR.PopNetBrowse()
		if !msgsHaveNetList(msgs) {
			s.syncNetListFallback()
		}
		s.AVR.SetNetUIBrowse(true)
		return nil
	case "net_down":
		if mock {
			s.AVR.SetNetUIBrowse(true)
			return nil
		}
		msgs, err := s.Client.ExchangeAll("NTCDOWN", 1200*time.Millisecond)
		if err != nil {
			return err
		}
		s.applyMsgs(msgs)
		if !msgsHaveNetList(msgs) {
			s.syncNetListFallback()
		}
		s.AVR.SetNetUIBrowse(true)
		return nil
	case "ntc":
		v := strings.ToUpper(strings.TrimSpace(r.FormValue("v")))
		switch v {
		case "PLAY", "PAUSE", "STOP", "TRUP", "TRDN":
		default:
			return fmt.Errorf("bad ntc %q", v)
		}
		if mock {
			return nil
		}
		msgs, err := s.Client.ExchangeAll("NTC"+v, 1200*time.Millisecond)
		if err != nil {
			return err
		}
		s.applyMsgs(msgs)
		s.syncNetMetaLight()
		if v == "STOP" {
			s.AVR.SetNetUIBrowse(true)
		} else if msgsNetPlaying(msgs) || state.IsNetPlaying(s.AVR.Snapshot().NetNST) {
			s.AVR.SetNetUIBrowse(false)
		}
		return nil
	case "pin_save":
		return s.pinSave(r)
	case "pin_play":
		return s.pinPlay(r, mock)
	case "pin_remove":
		return s.pinRemove(r)
	default:
		return fmt.Errorf("unknown action %q", action)
	}
}

func msgsHaveNetList(msgs []eiscp.Message) bool {
	for _, m := range msgs {
		if m.Cmd == "NLS" {
			if item, ok := state.ParseNLSItem(m.Param); ok && item.Text != "" {
				return true
			}
		}
		if m.Cmd == "NLT" && state.ParseNLTTitle(m.Param) != "" {
			return true
		}
	}
	return false
}

func msgsNetPlaying(msgs []eiscp.Message) bool {
	playing := false
	hasTitle := false
	for _, m := range msgs {
		switch m.Cmd {
		case "NST":
			if state.IsNetPlaying(m.Param) {
				playing = true
			}
		case "NTI":
			if strings.TrimSpace(m.Param) != "" {
				hasTitle = true
			}
		}
	}
	return playing && hasTitle
}

// msgsLookLikeNetPlayback is true when the amp returned a title (or play state)
// without menu U-rows — typical when a station starts (NST may still be S briefly).
func msgsLookLikeNetPlayback(msgs []eiscp.Message) bool {
	if msgsHaveNetList(msgs) {
		return false
	}
	hasTitle := false
	for _, m := range msgs {
		switch m.Cmd {
		case "NTI":
			if strings.TrimSpace(m.Param) != "" {
				hasTitle = true
			}
		case "NST":
			if state.IsNetPlaying(m.Param) {
				return true
			}
		}
	}
	return hasTitle
}

func (s *Server) applyMsgs(msgs []eiscp.Message) {
	for _, m := range msgs {
		s.AVR.ApplyLive(m.Cmd, m.Param)
	}
}

// syncNetListFallback only when the primary reply lacked usable list frames.
func (s *Server) syncNetListFallback() {
	if s.Client == nil {
		return
	}
	msgs, err := s.Client.ExchangeAll("NLTQSTN", 1500*time.Millisecond)
	if err == nil {
		s.applyMsgs(msgs)
	}
}

func (s *Server) syncNetMetaLight() {
	if s.Client == nil {
		return
	}
	for _, cmd := range []string{"NST", "NTI", "NAT"} {
		if q, err := s.Client.Query(cmd); err == nil && q != "" {
			s.AVR.ApplyLive(cmd, q)
		}
	}
}

func (s *Server) pinSave(r *http.Request) error {
	name := strings.TrimSpace(r.FormValue("name"))
	snap := s.AVR.Snapshot()
	if name == "" {
		name = snap.NetTitle
	}
	if name == "" {
		name = snap.NetListTitle
	}
	steps := snap.NetBrowsePath
	if len(steps) == 0 {
		return fmt.Errorf("nothing to pin — browse to a station first")
	}
	return s.Pins.Save(name, steps)
}

func (s *Server) pinRemove(r *http.Request) error {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return fmt.Errorf("missing pin name")
	}
	return s.Pins.Remove(name)
}

func (s *Server) pinPlay(r *http.Request, mock bool) error {
	name := strings.TrimSpace(r.FormValue("name"))
	pin, ok := s.Pins.Get(name)
	if !ok {
		return fmt.Errorf("unknown pin %q", name)
	}
	if mock {
		s.AVR.SetInput("2B")
		s.AVR.SetNetService("0e")
		s.AVR.SetMockNetTopMenu()
		for _, line := range pin.Steps {
			s.AVR.MockNetSelect(line)
		}
		return nil
	}
	if s.AVR.Snapshot().InputID != "2B" {
		if _, err := s.Client.Command("SLI", "2B"); err != nil {
			return err
		}
		s.AVR.SetInput("2B")
		time.Sleep(100 * time.Millisecond)
	}
	s.AVR.SetNetService("0e")
	s.AVR.ResetNetBrowse()
	msgs, err := s.Client.ExchangeAll("NSV"+state.NSVParam("0e"), 1500*time.Millisecond)
	if err != nil {
		return err
	}
	s.applyMsgs(msgs)
	if !msgsHaveNetList(msgs) {
		time.Sleep(120 * time.Millisecond)
		s.syncNetListFallback()
	}
	played := make([]int, 0, len(pin.Steps))
	for _, line := range pin.Steps {
		s.AVR.PushNetBrowse(line)
		played = append(played, line)
		m2, err := s.Client.ExchangeAll(fmt.Sprintf("NLSL%d", line), 2500*time.Millisecond)
		if err != nil {
			s.AVR.SetNetBrowsePath(played[:len(played)-1])
			return err
		}
		s.applyMsgs(m2)
		if msgsLookLikeNetPlayback(m2) || msgsNetPlaying(m2) {
			s.AVR.SetNetBrowsePath(played)
			s.AVR.SetNetUIBrowse(false)
			s.syncNetMetaLight()
			return nil
		}
		if !msgsHaveNetList(m2) {
			s.syncNetListFallback()
		}
	}
	s.AVR.SetNetBrowsePath(played)
	s.syncNetMetaLight()
	snap := s.AVR.Snapshot()
	if (state.IsNetPlaying(snap.NetNST) || snap.NetTitle != "") && len(snap.NetList) == 0 {
		s.AVR.SetNetUIBrowse(false)
	} else {
		s.AVR.SetNetUIBrowse(true)
	}
	return nil
}

func (s *Server) applyTUNResp(resp string) {
	if resp == "" {
		return
	}
	p := strings.TrimSpace(resp)
	if strings.EqualFold(p, "UP") || strings.EqualFold(p, "DOWN") || strings.EqualFold(p, "N/A") {
		return
	}
	// PRS replies are hex 01-28; TUN is 5-digit decimal. Prefer TUN-shaped.
	if _, err := strconv.Atoi(p); err == nil && len(p) >= 4 {
		s.AVR.ApplyLive("TUN", p)
	}
}

func (s *Server) syncTunerFreq() {
	if s.Client == nil {
		return
	}
	if q, err := s.Client.Query("TUN"); err == nil && q != "" {
		s.AVR.ApplyLive("TUN", q)
	}
}

func (s *Server) syncTuner() {
	if s.Client == nil {
		return
	}
	s.syncTunerFreq()
	if q, err := s.Client.Query("PRS"); err == nil && q != "" {
		s.AVR.ApplyLive("PRS", q)
	}
}

func (s *Server) syncAV(settle bool) {
	if s.Client == nil {
		return
	}
	if settle {
		time.Sleep(200 * time.Millisecond)
	}
	for _, cmd := range []string{"LMD", "IFA", "IFV"} {
		if q, err := s.Client.Query(cmd); err == nil && q != "" {
			s.AVR.ApplyLive(cmd, q)
		}
	}
}

func (s *Server) needsLoudConfirm(current, target float64) bool {
	if s.VolWarn <= 0 {
		return false
	}
	return current <= s.VolWarn && target > s.VolWarn
}

func (s *Server) loudOK(r *http.Request) bool {
	return r.FormValue("loud_ok") == "1"
}

func (s *Server) requireLoudOK(r *http.Request, current, target float64) error {
	if s.needsLoudConfirm(current, target) && !s.loudOK(r) {
		return fmt.Errorf("loudness confirm required (warn=%.1f target=%.1f)", s.VolWarn, target)
	}
	return nil
}

func (s *Server) volDelta(r *http.Request, d float64) error {
	cur := s.AVR.Snapshot().VolDisplay
	target := cur + d
	if err := s.requireLoudOK(r, cur, target); err != nil {
		return err
	}
	if s.Client == nil {
		s.AVR.NudgeVol(d)
		return nil
	}
	// Absolute MVL; UI step is 0.5 (one amp hex step). Display follows amp reply only.
	return s.setVolDisplay(target)
}

func (s *Server) setVolDisplay(v float64) error {
	resp, err := s.Client.Command("MVL", state.DisplayToHexVol(v))
	if err != nil {
		return err
	}
	if !liveVolParam(resp) {
		resp, err = s.Client.Query("MVL")
		if err != nil {
			return err
		}
	}
	if liveVolParam(resp) {
		s.AVR.ApplyLive("MVL", resp)
	}
	return nil
}

func (s *Server) ctlDelta(d float64) error {
	if s.Client == nil {
		s.AVR.NudgeCTL(d)
		return nil
	}
	// Absolute CTL wire write; display follows amp reply only.
	return s.setCTL(s.AVR.Snapshot().CTL + d)
}

func (s *Server) setCTL(n float64) error {
	resp, err := s.Client.Command("CTL", state.FormatCTLParam(n))
	if err != nil {
		return err
	}
	if !liveCTLParam(resp) {
		resp, err = s.Client.Query("CTL")
		if err != nil {
			return err
		}
	}
	if liveCTLParam(resp) {
		s.AVR.ApplyLive("CTL", resp)
	}
	return nil
}

func liveVolParam(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || strings.EqualFold(p, "QSTN") || strings.EqualFold(p, "UP") || strings.EqualFold(p, "DOWN") {
		return false
	}
	_, err := strconv.ParseInt(p, 16, 64)
	return err == nil
}

func liveCTLParam(p string) bool {
	_, ok := state.CTLWireToDb(p)
	return ok
}
