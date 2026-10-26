//go:build integration

package main

import (
	"strings"
	"testing"
	"time"
)

func TestIntegrationBudgetExpiryIs504WithoutRetry(t *testing.T) {
	ups := upstreams(t)
	ups["PRODUCT_DISCOVERY_URL"].delay = 2 * time.Second
	s := &session{t: t, base: bff(t, ups, map[string]string{"BUDGET_QUERY_MS": "100"})}
	start := time.Now()
	code, body := s.do("GET", "/products/p1", "")
	if code != 504 || !strings.Contains(body, "/problems/gateway-timeout") {
		t.Fatalf("got %d %s", code, body)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("the budget was not honoured: %s", time.Since(start))
	}
	time.Sleep(300 * time.Millisecond)
	if n := len(ups["PRODUCT_DISCOVERY_URL"].requests()); n != 1 {
		t.Fatalf("a timed out call must not be retried, upstream saw %d requests", n)
	}
}

func TestIntegrationUpstreamProblemsPassThrough(t *testing.T) {
	ups := upstreams(t)
	s := &session{t: t, base: bff(t, ups, nil)}
	code, body := s.do("GET", "/carts/unknown", "")
	if code != 404 || !strings.Contains(body, "/problems/not-found") {
		t.Fatalf("got %d %s", code, body)
	}
}

func TestIntegrationUnreachableUpstreamIs503(t *testing.T) {
	ups := upstreams(t)
	ups["PRODUCT_DISCOVERY_URL"].srv.Close()
	s := &session{t: t, base: bff(t, ups, nil)}
	if code, _ := s.do("GET", "/products/p1", ""); code != 503 {
		t.Fatalf("got %d, want 503", code)
	}
}
