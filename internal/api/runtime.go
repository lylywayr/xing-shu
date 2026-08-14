package api

import (
	"sync"
	"time"
	"xing-shu/internal/runtime"
)

type Runtime struct {
	Cache             *runtime.Cache
	Sessions          *runtime.Sessions
	Learning          *runtime.Learning
	Reviews           *runtime.Reviews
	ReviewSession     *runtime.ReviewSession
	ReviewerSelection *runtime.ReviewerSelection
	Knowledge         *runtime.KnowledgeStore
	Shadow            *runtime.ShadowStore
	ShadowBatches     *runtime.ShadowBatches
	mu                sync.Mutex
}

func NewRuntime() *Runtime {
	return NewRuntimeAt("/data")
}
func NewRuntimeAt(dir string) *Runtime {
	c := runtime.NewCache()
	c.Load(dir + "/cache-xing-shu.json")
	s := runtime.NewSessions()
	s.Load(dir + "/sessions-xing-shu.json")
	l := runtime.NewLearning()
	l.Load(dir + "/learning-xing-shu.json")
	r := runtime.NewReviews()
	r.Load(dir + "/reviews-xing-shu.json")
	r.RecoverProcessing(2 * time.Hour)
	rs := runtime.NewReviewSession()
	rs.Load(dir + "/reviewer-session-xing-shu.json")
	sel := runtime.NewReviewerSelection()
	sel.Load(dir + "/reviewer-selection-xing-shu.json")
	ks := runtime.NewKnowledgeStore()
	ks.Load(dir + "/routing-knowledge-xing-shu.json")
	sh := runtime.NewShadowStore()
	sh.Load(dir + "/shadow-results-xing-shu.json")
	batches := runtime.NewShadowBatches()
	batches.Load(dir + "/shadow-batches-xing-shu.json")
	batches.RecoverInterrupted()
	return &Runtime{Cache: c, Sessions: s, Learning: l, Reviews: r, ReviewSession: rs, ReviewerSelection: sel, Knowledge: ks, Shadow: sh, ShadowBatches: batches}
}
