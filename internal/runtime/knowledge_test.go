package runtime

import (
	"testing"
	"time"
)

func TestRecordValidationPersistsHistoryAndRejectsDuplicate(t *testing.T) {
	store := NewKnowledgeStore()
	store.Upsert(Knowledge{ID: "k1", Status: KnowledgeProposed})
	if !store.RecordValidation("k1", "v1", false, CauseProtocol, "bad response") {
		t.Fatal("first validation should be recorded")
	}
	if store.RecordValidation("k1", "v1", true, "", "duplicate") {
		t.Fatal("duplicate validation should be rejected")
	}
	item := store.All()[0]
	if len(item.ValidationHistory) != 1 || item.FailedSamples != 1 || item.FailureCause != CauseProtocol {
		t.Fatalf("validation history not persisted: %+v", item)
	}
	if item.ValidationHistory[0].StatusFrom != KnowledgeProposed || item.ValidationHistory[0].StatusTo != KnowledgeProposed {
		t.Fatalf("unexpected status transition: %+v", item.ValidationHistory[0])
	}
}

func TestExpiredKnowledgeCanBeRestoredToValidated(t *testing.T) {
	store := NewKnowledgeStore()
	expires := time.Now().Add(-time.Hour)
	store.Upsert(Knowledge{ID: "k1", Status: KnowledgeTrusted, ExpiresAt: &expires})
	if store.Expire(time.Now()) != 1 {
		t.Fatal("expected one expired knowledge item")
	}
	if !store.Restore("k1", KnowledgeValidated, "revalidated by operator") {
		t.Fatal("expired knowledge should be restorable")
	}
	item := store.All()[0]
	if item.Status != KnowledgeValidated || item.ExpiresAt == nil || !item.ExpiresAt.After(time.Now()) {
		t.Fatalf("restore did not renew lifecycle: %+v", item)
	}
}
