package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	Catalog Catalog              `json:"catalog"`
	Allow   map[string]bool      `json:"allow"`
	Last    map[string]time.Time `json:"last_sync"`
}

func EmptyState() State {
	return State{Catalog: Catalog{Models: []Model{}}, Allow: map[string]bool{}, Last: map[string]time.Time{}}
}

func LoadState(path string) (State, error) {
	state := EmptyState()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return EmptyState(), err
	}
	if state.Catalog.Models == nil {
		state.Catalog.Models = []Model{}
	}
	if state.Allow == nil {
		state.Allow = map[string]bool{}
	}
	if state.Last == nil {
		state.Last = map[string]time.Time{}
	}
	return state, nil
}

func SaveState(path string, state State) error {
	if state.Catalog.Models == nil {
		state.Catalog.Models = []Model{}
	}
	if state.Allow == nil {
		state.Allow = map[string]bool{}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
