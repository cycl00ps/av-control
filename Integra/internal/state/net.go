package state

import (
	"fmt"
	"strconv"
	"strings"
)

// NetService is one entry from NRI netservicelist (NSV id).
type NetService struct {
	ID   string // e.g. "0e"
	Name string
	Kind string // "tunein" | "sink" | "app" | "local"
	Hint string // shown for sink services
}

// FallbackNetServices matches the live DRX-3.4 NRI dump.
var FallbackNetServices = []NetService{
	{ID: "0e", Name: "TuneIn Radio", Kind: "tunein"},
	{ID: "0a", Name: "Spotify", Kind: "sink", Hint: "Open Spotify on your phone and Connect to this Integra."},
	{ID: "12", Name: "Deezer", Kind: "app"},
	{ID: "44", Name: "AirPlay", Kind: "sink", Hint: "Open AirPlay on your phone/Mac and pick this Integra."},
	{ID: "1b", Name: "TIDAL", Kind: "app"},
	{ID: "00", Name: "Music Server", Kind: "local"},
	{ID: "f0", Name: "USB", Kind: "local"},
	{ID: "40", Name: "Chromecast", Kind: "sink", Hint: "Cast from your phone to this Integra."},
	{ID: "1d", Name: "Play Queue", Kind: "local"},
	{ID: "42", Name: "DTS Play-Fi", Kind: "sink", Hint: "Use a Play-Fi app to stream to this Integra."},
	{ID: "45", Name: "Amazon Alexa", Kind: "sink", Hint: "Use Alexa to play to this Integra."},
}

// NetListItem is one visible NET list row (NLS U-line).
type NetListItem struct {
	Line int
	Text string
}

// NSVParam builds the NSV wire param: 2-char service id + zone digit 0.
func NSVParam(serviceID string) string {
	id := strings.ToLower(strings.TrimSpace(serviceID))
	if len(id) == 1 {
		id = "0" + id
	}
	if len(id) > 2 {
		id = id[:2]
	}
	return id + "0"
}

// ParseNLTTitle extracts the menu title after the 22-char hex header.
func ParseNLTTitle(param string) string {
	p := strings.TrimSpace(param)
	if len(p) <= 22 {
		return ""
	}
	// Header is 11 bytes as hex (22 chars) when well-formed.
	hdr := p[:22]
	ok := true
	for _, c := range hdr {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			ok = false
			break
		}
	}
	if ok {
		return strings.TrimSpace(p[22:])
	}
	return strings.TrimSpace(p)
}

// ParseNLSItem parses "U0-My Presets" into a list row.
func ParseNLSItem(param string) (NetListItem, bool) {
	p := strings.TrimSpace(param)
	if len(p) < 3 || (p[0] != 'U' && p[0] != 'u') {
		return NetListItem{}, false
	}
	rest := p[1:]
	dash := strings.IndexByte(rest, '-')
	if dash < 1 {
		return NetListItem{}, false
	}
	line, err := strconv.ParseInt(rest[:dash], 16, 64)
	if err != nil {
		return NetListItem{}, false
	}
	return NetListItem{Line: int(line), Text: rest[dash+1:]}, true
}

// NetPlayStateLabel maps NST first letter to UI text.
func NetPlayStateLabel(nst string) string {
	if nst == "" {
		return ""
	}
	switch nst[0] {
	case 'P':
		return "Playing"
	case 'p':
		return "Paused"
	case 'S', 's':
		return "Stopped"
	case 'F', 'f':
		return "FF"
	case 'R', 'r':
		return "REW"
	default:
		return nst
	}
}

// IsNetPlaying reports whether NST indicates active playback (play or pause).
func IsNetPlaying(nst string) bool {
	if nst == "" {
		return false
	}
	return nst[0] == 'P' || nst[0] == 'p'
}

// FindNetService returns a known service by id (case-insensitive).
func FindNetService(id string) (NetService, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, s := range FallbackNetServices {
		if strings.ToLower(s.ID) == id {
			return s, true
		}
	}
	return NetService{}, false
}

// FormatNetServiceButtonLabel shortens long Chromecast name for the grid.
func FormatNetServiceButtonLabel(name string) string {
	if strings.HasPrefix(name, "Chromecast") {
		return "Chromecast"
	}
	return name
}

// NetServiceKind returns kind for id.
func NetServiceKind(id string) string {
	if s, ok := FindNetService(id); ok {
		return s.Kind
	}
	return "app"
}

func NetServiceHint(id string) string {
	if s, ok := FindNetService(id); ok {
		return s.Hint
	}
	return ""
}

func NetServiceName(id string) string {
	if s, ok := FindNetService(id); ok {
		return s.Name
	}
	return fmt.Sprintf("Service %s", id)
}
