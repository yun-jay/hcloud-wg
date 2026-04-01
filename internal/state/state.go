package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ServerEntry struct {
	Name        string    `json:"name"`
	HCloudID    int       `json:"hcloud_id"`
	PublicIP    string    `json:"public_ip"`
	WGIP        string    `json:"wg_ip"`
	WGPublicKey string    `json:"wg_public_key"`
	ServerType  string    `json:"server_type"`
	CreatedAt   time.Time `json:"created_at"`
}

type Store struct {
	path    string
	entries []ServerEntry
}

func Load(path string) (*Store, error) {
	s := &Store{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	if err := json.Unmarshal(data, &s.entries); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	return s, nil
}

func (s *Store) Save() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("renaming state file: %w", err)
	}

	return nil
}

func (s *Store) Add(entry ServerEntry) error {
	s.entries = append(s.entries, entry)
	return s.Save()
}

func (s *Store) Remove(name string) (*ServerEntry, bool) {
	for i, e := range s.entries {
		if e.Name == name {
			removed := s.entries[i]
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			s.Save()
			return &removed, true
		}
	}
	return nil, false
}

func (s *Store) FindByName(name string) *ServerEntry {
	for i := range s.entries {
		if s.entries[i].Name == name {
			return &s.entries[i]
		}
	}
	return nil
}

func (s *Store) NextWGIP(rangeStart, rangeEnd int) (string, error) {
	used := make(map[int]bool)
	for _, e := range s.entries {
		var octet int
		fmt.Sscanf(e.WGIP, "10.0.0.%d", &octet)
		used[octet] = true
	}

	for i := rangeStart; i <= rangeEnd; i++ {
		if !used[i] {
			return fmt.Sprintf("10.0.0.%d", i), nil
		}
	}

	return "", fmt.Errorf("no available WireGuard IPs in range %d-%d", rangeStart, rangeEnd)
}

func (s *Store) Prune(activeIDs map[int]bool) int {
	var kept []ServerEntry
	removed := 0

	for _, e := range s.entries {
		if activeIDs[e.HCloudID] {
			kept = append(kept, e)
		} else {
			removed++
		}
	}

	if removed > 0 {
		s.entries = kept
		s.Save()
	}

	return removed
}

func (s *Store) All() []ServerEntry {
	return s.entries
}
