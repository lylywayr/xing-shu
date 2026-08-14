package api

import (
	"context"
	"time"
)

func StartMaintenance(ctx context.Context, rt *Runtime) {
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cut := time.Now().Add(-90 * 24 * time.Hour)
				if rt != nil {
					if rt.Reviews != nil {
						rt.Reviews.Prune(cut)
					}
					if rt.Shadow != nil {
						rt.Shadow.Prune(cut)
					}
					if rt.Knowledge != nil {
						rt.Knowledge.Expire(time.Now())
					}
				}
			}
		}
	}()
}
