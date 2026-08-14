package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"xing-shu/internal/governance"
)

func LoadGovernance(dir string) ([]governance.Record, error) {
	path := filepath.Join(dir, "governance-starcore.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []governance.Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records []governance.Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []governance.Record{}
	}
	return records, nil
}

func SaveGovernance(dir string, records []governance.Record) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "governance-starcore.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
