package catalog

import (
	"context"
	"log"
	"time"
	"xing-shu/internal/provider"
)

type SyncObserver func(providerID string, result provider.Result)
type SyncGate func(providerID string) bool

func StartSync(ctx context.Context, m *Manager, configs map[string]provider.Config, interval time.Duration) {
	StartSyncWithObserverGated(ctx, m, configs, interval, nil, nil)
}

func StartSyncWithObserver(ctx context.Context, m *Manager, configs map[string]provider.Config, interval time.Duration, observer SyncObserver) {
	StartSyncWithObserverGated(ctx, m, configs, interval, observer, nil)
}

func StartSyncWithSource(ctx context.Context, m *Manager, source func() map[string]provider.Config, interval time.Duration, observer SyncObserver, gate SyncGate) {
	go func() {
		run := func() {
			for _, c := range source() {
				if c.BaseURL == "" || c.Source == provider.SourceExternal || (gate != nil && !gate(c.ID)) {
					continue
				}
				result := m.Sync(ctx, c)
				if observer != nil {
					observer(c.ID, result)
				}
				if result.ErrorType != "" {
					log.Printf("provider=%s sync_error=%s msg=%s", c.ID, result.ErrorType, result.Message)
				}
			}
		}
		run()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}

func StartSyncWithObserverGated(ctx context.Context, m *Manager, configs map[string]provider.Config, interval time.Duration, observer SyncObserver, gate SyncGate) {
	StartSyncWithSource(ctx, m, func() map[string]provider.Config { return configs }, interval, observer, gate)
}
