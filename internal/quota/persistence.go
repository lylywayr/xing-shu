package quota

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type persistedState struct {
	Items   []Snapshot `json:"items"`
	History []Snapshot `json:"history"`
}

func (m *Manager) Save(path string) error {
	if m == nil || path == "" {
		return errors.New("quota state path unavailable")
	}
	state := persistedState{Items: m.All(), History: m.History()}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func (m *Manager) Load(path string) error {
	if m == nil || path == "" {
		return errors.New("quota state path unavailable")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state persistedState
	if err := json.Unmarshal(body, &state); err != nil {
		return err
	}
	items := map[string]Snapshot{}
	for _, item := range state.Items {
		if item.Provider != "" {
			items[snapshotKey(item)] = item
		}
	}
	for _, item := range state.History {
		if item.Provider != "" {
			if _, exists := items[snapshotKey(item)]; !exists {
				items[snapshotKey(item)] = item
			}
		}
	}
	if state.History == nil {
		state.History = state.Items
	}
	if len(state.History) > 200 {
		state.History = state.History[len(state.History)-200:]
	}
	m.mu.Lock()
	m.items = items
	m.history = append([]Snapshot(nil), state.History...)
	m.mu.Unlock()
	return nil
}
