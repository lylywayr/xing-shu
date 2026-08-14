package runtime

import "time"

func (r *Reviews) Prune(before time.Time) int {
	n := 0
	r.mu.Lock()
	for id, x := range r.items {
		if x.RecordedAt.Before(before) {
			delete(r.items, id)
			n++
		}
	}
	r.mu.Unlock()
	if n > 0 {
		r.persist()
	}
	return n
}
func (s *ShadowStore) Prune(before time.Time) int {
	n := 0
	s.mu.Lock()
	for id, x := range s.items {
		if x.CreatedAt.Before(before) {
			delete(s.items, id)
			n++
		}
	}
	s.mu.Unlock()
	if n > 0 {
		s.persist()
	}
	return n
}
