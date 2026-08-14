package provider

import (
	"os"
	"sync"
)

var externalMu sync.RWMutex
var externalConfigs = map[string]Config{}

func RegisterExternalConfig(id string, config Config) {
	if id == "" || config.BaseURL == "" {
		return
	}
	externalMu.Lock()
	externalConfigs[id] = config
	externalMu.Unlock()
}

func UnregisterExternalConfig(id string) {
	externalMu.Lock()
	delete(externalConfigs, id)
	externalMu.Unlock()
}

// EnvConfig describes one optional OpenAI-compatible Provider. The public
// build intentionally has no embedded endpoint, credential, or LAN address.
func EnvConfig(id, url, key, kind string) Config {
	return Config{ID: id, BaseURL: os.Getenv(url), APIKey: os.Getenv(key), Kind: kind}
}

func LoadConfigs() map[string]Config {
	configs := map[string]Config{
		"provider-a": EnvConfig("provider-a", "PROVIDER_A_URL", "PROVIDER_A_KEY", "standard"),
		"provider-b": EnvConfig("provider-b", "PROVIDER_B_URL", "PROVIDER_B_KEY", "standard"),
		"provider-c": EnvConfig("provider-c", "PROVIDER_C_URL", "PROVIDER_C_KEY", "standard"),
		"workbuddy":  EnvConfig("workbuddy", "WORKBUDDY_URL", "WORKBUDDY_KEY", "standard"),
		"trae":       EnvConfig("trae", "TRAE_URL", "TRAE_KEY", "standard"),
		"qoder":      EnvConfig("qoder", "QODER_URL", "QODER_KEY", "standard"),
		"cctq":       EnvConfig("cctq", "CCTQ_URL", "CCTQ_KEY", "standard"),
	}
	for id, cfg := range configs {
		if cfg.BaseURL == "" {
			delete(configs, id)
		}
	}
	externalMu.RLock()
	for id, cfg := range externalConfigs {
		configs[id] = cfg
	}
	externalMu.RUnlock()
	return configs
}

func LoadConfigsWithExternal(free map[string]Config) map[string]Config {
	configs := LoadConfigs()
	for id, cfg := range free {
		configs[id] = cfg
	}
	return configs
}
