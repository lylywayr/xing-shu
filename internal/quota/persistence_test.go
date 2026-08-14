package quota

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerPersistsAndLoadsFacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota-history.json")
	m := NewManager()
	remaining, used, limit := 5.0, 95.0, 100.0
	m.Set(Snapshot{Provider: "newapi", State: Available, Remaining: &remaining, Used: &used, Limit: &limit, CheckedAt: time.Now().UTC()})
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded := NewManager()
	if err := loaded.Load(path); err != nil {
		t.Fatal(err)
	}
	items := loaded.All()
	if len(items) != 1 || items[0].Provider != "newapi" || items[0].Remaining == nil || *items[0].Remaining != 5 {
		t.Fatalf("unexpected loaded quota: %+v", items)
	}
	if len(loaded.History()) != 1 || loaded.History()[0].Used == nil || *loaded.History()[0].Used != 95 {
		t.Fatalf("unexpected loaded history: %+v", loaded.History())
	}
}

func TestManagerLoadMalformedDoesNotEraseState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	if err := m.Load(path); err == nil {
		t.Fatal("malformed quota state should fail")
	}
	if len(m.All()) != 0 || len(m.History()) != 0 {
		t.Fatal("malformed load mutated manager")
	}
}
