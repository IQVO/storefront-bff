package config

import (
	"strings"
	"testing"
	"time"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

func env(extra map[string]string) func(string) string {
	m := map[string]string{}
	for _, name := range UpstreamEnv {
		m[name] = "http://" + strings.ToLower(name) + ":8080"
	}
	for k, v := range extra {
		m[k] = v
	}
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || !cfg.CookieSecure || cfg.Budgets != composition.DefaultBudgets() || len(cfg.Upstreams) != 6 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{"PORT": "9000", "COOKIE_SECURE": "false", "BUDGET_QUERY_MS": "500", "BUDGET_COMMAND_MS": "1500", "BUDGET_CONFIRM_MS": "30000"}))
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Budgets
	if cfg.Addr != ":9000" || cfg.CookieSecure || b.Query != 500*time.Millisecond || b.Command != 1500*time.Millisecond || b.Confirm != 30*time.Second {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadReportsEveryMissingUpstream(t *testing.T) {
	_, err := Load(func(string) string { return "" })
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, name := range UpstreamEnv {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %s: %v", name, err)
		}
	}
}

func TestLoadRejectsBadOptionalValues(t *testing.T) {
	for _, extra := range []map[string]string{
		{"BUDGET_QUERY_MS": "0"}, {"BUDGET_COMMAND_MS": "x"}, {"BUDGET_CONFIRM_MS": "-5"}, {"COOKIE_SECURE": "maybe"},
	} {
		if _, err := Load(env(extra)); err == nil {
			t.Errorf("%v must be rejected", extra)
		}
	}
}
