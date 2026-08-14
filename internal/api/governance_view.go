package api

import (
	"time"
	"xing-shu/internal/catalog"
	"xing-shu/internal/governance"
)

func reconcileGovernance(c catalog.Catalog, records []governance.Record) []governance.Record {
	byKey := make(map[string]governance.Record, len(records))
	for _, record := range records {
		byKey[record.Key] = record
	}
	for _, model := range c.Models {
		key := model.Provider + "/" + model.ID
		if _, exists := byKey[key]; exists {
			continue
		}
		byKey[key] = governance.Record{Key: key, Model: model, Status: catalog.Unknown, Reason: "catalog model has no governance record", CheckedAt: time.Now(), LastSeen: time.Now()}
	}
	out := make([]governance.Record, 0, len(byKey))
	for _, record := range byKey {
		out = append(out, record)
	}
	return out
}
