package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TuneInPin is a one-tap replay path from TuneIn root (NSV0e0).
type TuneInPin struct {
	Name  string `json:"name"`
	Steps []int  `json:"steps"` // NLSL line indices from root
}

type PinStore struct {
	mu   sync.Mutex
	path string
	pins []TuneInPin
}

func NewPinStore(path string) *PinStore {
	s := &PinStore{path: path}
	_ = s.Load()
	return s
}

func (s *PinStore) Path() string { return s.path }

func (s *PinStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.pins = nil
			return nil
		}
		return err
	}
	var pins []TuneInPin
	if err := json.Unmarshal(b, &pins); err != nil {
		return err
	}
	s.pins = sanitizePins(pins)
	return nil
}

func (s *PinStore) List() []TuneInPin {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TuneInPin, len(s.pins))
	copy(out, s.pins)
	for i := range out {
		out[i].Steps = append([]int(nil), s.pins[i].Steps...)
	}
	return out
}

func (s *PinStore) Save(name string, steps []int) error {
	name = strings.TrimSpace(name)
	if name == "" || len(steps) == 0 {
		return errPinInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	steps = append([]int(nil), steps...)
	for i, p := range s.pins {
		if strings.EqualFold(p.Name, name) {
			s.pins[i] = TuneInPin{Name: name, Steps: steps}
			return s.persistLocked()
		}
	}
	s.pins = append(s.pins, TuneInPin{Name: name, Steps: steps})
	return s.persistLocked()
}

func (s *PinStore) Remove(name string) error {
	name = strings.TrimSpace(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.pins[:0]
	for _, p := range s.pins {
		if !strings.EqualFold(p.Name, name) {
			out = append(out, p)
		}
	}
	s.pins = out
	return s.persistLocked()
}

func (s *PinStore) Get(name string) (TuneInPin, bool) {
	name = strings.TrimSpace(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pins {
		if strings.EqualFold(p.Name, name) {
			return TuneInPin{Name: p.Name, Steps: append([]int(nil), p.Steps...)}, true
		}
	}
	return TuneInPin{}, false
}

func (s *PinStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.pins, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func sanitizePins(in []TuneInPin) []TuneInPin {
	out := make([]TuneInPin, 0, len(in))
	for _, p := range in {
		name := strings.TrimSpace(p.Name)
		if name == "" || len(p.Steps) == 0 {
			continue
		}
		steps := make([]int, 0, len(p.Steps))
		for _, n := range p.Steps {
			if n < 0 || n > 9 {
				continue
			}
			steps = append(steps, n)
		}
		if len(steps) == 0 {
			continue
		}
		out = append(out, TuneInPin{Name: name, Steps: steps})
	}
	return out
}

// Sentinel for empty pin payloads (avoid importing fmt/errors only for this).
type pinError string

func (e pinError) Error() string { return string(e) }

const errPinInvalid pinError = "invalid pin"
