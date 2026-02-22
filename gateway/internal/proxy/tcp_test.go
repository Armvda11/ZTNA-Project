package proxy

import (
	"log/slog"
	"os"
	"testing"

	"gateway/internal/config"
)

func TestNewTCPProxy(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			DialTimeout: "10s",
			MaxConns:    1000,
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	proxy := NewTCPProxy(cfg, log)

	if proxy == nil {
		t.Fatal("NewTCPProxy() returned nil")
	}
	if proxy.cfg != cfg {
		t.Error("NewTCPProxy() did not store config")
	}
	if proxy.log != log {
		t.Error("NewTCPProxy() did not store logger")
	}
}
