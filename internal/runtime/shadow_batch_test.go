package runtime

import "testing"

func TestShadowBatchesLoadStopsInterruptedWork(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/batches.json"
	store := NewShadowBatches()
	store.Load(path)
	store.Put(ShadowBatch{ID: "b1", Status: ShadowBatchRunning, Total: 3, Completed: 1})
	store2 := NewShadowBatches()
	store2.Load(path)
	item, ok := store2.Get("b1")
	if !ok || item.Status != ShadowBatchStopped || item.LastError == "" {
		t.Fatalf("interrupted batch not stopped: %+v", item)
	}
}

func TestShadowBatchesStopCountsRemaining(t *testing.T) {
	store := NewShadowBatches()
	store.Put(ShadowBatch{ID: "b1", Status: ShadowBatchRunning, Total: 4, Completed: 1})
	if !store.Stop("b1") {
		t.Fatal("stop should find batch")
	}
	item, _ := store.Get("b1")
	if item.Status != ShadowBatchStopped || item.Stopped != 3 {
		t.Fatalf("unexpected stopped count: %+v", item)
	}
}
