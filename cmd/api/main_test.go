package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunRefusesToBootWithoutUpstreams(t *testing.T) {
	err := run(context.Background(), func(string) string { return "" }, quietLog(), nil)
	if err == nil || !strings.Contains(err.Error(), "PRODUCT_DISCOVERY_URL") {
		t.Fatalf("err = %v", err)
	}
}

// TestRunServesAndShutsDown wires the real binary against a fake upstream.
func TestRunServesAndShutsDown(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"from":"`+r.URL.Path+`"}`)
	}))
	defer up.Close()
	getenv := func(k string) string {
		switch {
		case k == "PORT":
			return "0"
		case strings.HasSuffix(k, "_URL"):
			return up.URL
		}
		return ""
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan string, 1), make(chan error, 1)
	go func() { done <- run(ctx, getenv, quietLog(), ready) }()
	addr := <-ready
	resp, err := http.Get("http://" + addr + "/api/storefront/products/p1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != `{"from":"/products/p1"}` {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not stop")
	}
}

func TestRunFailsWhenPortIsTaken(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	getenv := func(k string) string {
		if k == "PORT" {
			return port
		}
		if strings.HasSuffix(k, "_URL") {
			return "http://127.0.0.1:1"
		}
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := run(ctx, getenv, quietLog(), nil); err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("err = %v", err)
	}
}
