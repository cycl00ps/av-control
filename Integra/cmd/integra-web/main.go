package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cycl00ps/av-control/Integra/internal/eiscp"
	"github.com/cycl00ps/av-control/Integra/internal/state"
	"github.com/cycl00ps/av-control/Integra/internal/web"
)

func main() {
	loadDotEnv(".env")
	loadDotEnv("Integra/.env")

	mock := envBool("INTEGRA_MOCK", false)
	addr := env("INTEGRA_HTTP_ADDR", "0.0.0.0:8080")
	host := env("INTEGRA_ADDRESS", "192.168.210.114")
	port := envInt("INTEGRA_EISCP_PORT", eiscp.DefaultPort)

	defs := state.Defaults{
		ListeningMode: env("listening_mode", "40"),
		MasterVolume:  env("master_volume", "50"),
		CenterLevel:   env("center_level", "+10"),
		LateNight:     env("late_night", "01"),
	}

	avr := state.New(mock, defs)
	var client *eiscp.Client
	if !mock {
		client = eiscp.NewClient(host, port)
		defer client.Close()
		syncLive(avr, client)
		go pollLive(avr, client)
	}

	base := env("INTEGRA_HTTP_BASE", "")
	volWarn := envFloat("INTEGRA_VOL_WARN", 55)
	srv, err := web.New(avr, client, base, volWarn)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	static := http.FileServer(http.Dir(findStatic()))
	srv.Routes(mux, static)

	log.Printf("integra-web listening on %s (mock=%v vol_warn=%.1f)", addr, mock, volWarn)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func syncLive(avr *state.AVR, c *eiscp.Client) {
	for _, cmd := range []string{"PWR", "AMT", "MVL", "SLI", "LMD", "CTL", "LTN", "DSS", "EQS", "TPD", "IFA", "IFV", "TUN", "PRS"} {
		param, err := c.Query(cmd)
		if err != nil || param == "" {
			continue
		}
		avr.ApplyLive(cmd, param)
	}
}

func pollLive(avr *state.AVR, c *eiscp.Client) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for range t.C {
		cmds := []string{"PWR", "AMT", "MVL", "SLI", "LMD", "TPD", "IFA", "IFV"}
		snap := avr.Snapshot()
		if snap.IsTuner {
			cmds = append(cmds, "TUN", "PRS")
		}
		for _, cmd := range cmds {
			param, err := c.Query(cmd)
			if err != nil || param == "" {
				continue
			}
			avr.ApplyLive(cmd, param)
		}
		if !snap.IsNet {
			continue
		}
		// While browsing, skip NET meta queries so poll does not hold the eISCP mutex
		// or re-assert NST/NTI and fight the browse UI.
		if snap.NetUIBrowse {
			if len(snap.NetList) == 0 && snap.NetServiceID != "" {
				msgs, err := c.ExchangeAll("NLTQSTN", 1200*time.Millisecond)
				if err == nil {
					for _, m := range msgs {
						avr.ApplyLive(m.Cmd, m.Param)
					}
				}
			}
			continue
		}
		for _, cmd := range []string{"NST", "NTI"} {
			param, err := c.Query(cmd)
			if err != nil || param == "" {
				continue
			}
			avr.ApplyLive(cmd, param)
		}
	}
}

func findStatic() string {
	for _, p := range []string{"static", "Integra/static"} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return "static"
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(k string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return n
}

// Minimal .env loader (KEY=VALUE lines); does not override existing env.
func loadDotEnv(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}
