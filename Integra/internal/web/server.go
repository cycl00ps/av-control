package web

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cycl00ps/av-control/Integra/internal/eiscp"
	"github.com/cycl00ps/av-control/Integra/internal/state"
)

//go:embed templates/*.html
var tmplFS embed.FS

type Server struct {
	AVR    *state.AVR
	Client *eiscp.Client // nil if mock
	Tmpl   *template.Template
}

type pageData struct {
	state.Snapshot
	Inputs []state.Input
}

func New(avr *state.AVR, client *eiscp.Client) (*Server, error) {
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
	return &Server{AVR: avr, Client: client, Tmpl: t}, nil
}

func (s *Server) data() pageData {
	return pageData{Snapshot: s.AVR.Snapshot(), Inputs: state.Inputs}
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
		return s.volDelta(1)
	case "vol_down":
		return s.volDelta(-1)
	case "vol_set":
		v, _ := strconv.ParseFloat(r.FormValue("value"), 64)
		if mock {
			s.AVR.SetVolDisplay(v)
			return nil
		}
		_, err := s.Client.Command("MVL", state.DisplayToHexVol(v))
		if err == nil {
			s.AVR.SetVolDisplay(v)
		}
		return err
	case "defaults":
		s.AVR.ApplyDefaults()
		if mock {
			return nil
		}
		d := s.AVR.Defaults
		_, _ = s.Client.Command("LMD", d.ListeningMode)
		time.Sleep(60 * time.Millisecond)
		_, _ = s.Client.Command("MVL", d.MasterVolume)
		time.Sleep(60 * time.Millisecond)
		_, _ = s.Client.Command("CTL", state.FormatCTLParam(state.ParseCTL(d.CenterLevel)))
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
		if err == nil {
			s.AVR.SetInput(id)
		}
		return err
	case "lmd":
		v := r.FormValue("v")
		if mock {
			s.AVR.SetLMD(v)
			return nil
		}
		_, err := s.Client.Command("LMD", v)
		if err == nil {
			s.AVR.SetLMD(v)
		}
		return err
	case "ctl_set":
		n, _ := strconv.Atoi(r.FormValue("value"))
		if mock {
			s.AVR.SetCTL(n)
			return nil
		}
		_, err := s.Client.Command("CTL", state.FormatCTLParam(n))
		if err == nil {
			s.AVR.SetCTL(n)
		}
		return err
	case "ctl_up":
		return s.ctlDelta(1)
	case "ctl_down":
		return s.ctlDelta(-1)
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
		_, err := s.Client.Command("TUN", "UP")
		return err
	case "tune_down":
		if mock {
			s.AVR.TuneNudge(false)
			return nil
		}
		_, err := s.Client.Command("TUN", "DOWN")
		return err
	case "preset":
		n, _ := strconv.Atoi(r.FormValue("n"))
		if mock {
			s.AVR.SetPreset(n)
			return nil
		}
		_, err := s.Client.Command("PRS", fmt.Sprintf("%02X", n))
		if err == nil {
			s.AVR.SetPreset(n)
		}
		return err
	case "band":
		id := r.FormValue("id")
		if mock {
			s.AVR.SetInput(id)
			return nil
		}
		_, err := s.Client.Command("SLI", id)
		if err == nil {
			s.AVR.SetInput(id)
		}
		return err
	default:
		return fmt.Errorf("unknown action %q", action)
	}
}

func (s *Server) volDelta(d float64) error {
	if s.Client == nil {
		s.AVR.NudgeVol(d)
		return nil
	}
	param := "UP"
	if d < 0 {
		param = "DOWN"
	}
	_, err := s.Client.Command("MVL", param)
	if err == nil {
		s.AVR.NudgeVol(d)
	}
	return err
}

func (s *Server) ctlDelta(d int) error {
	if s.Client == nil {
		s.AVR.NudgeCTL(d)
		return nil
	}
	param := "UP"
	if d < 0 {
		param = "DOWN"
	}
	_, err := s.Client.Command("CTL", param)
	if err == nil {
		s.AVR.NudgeCTL(d)
	}
	return err
}
