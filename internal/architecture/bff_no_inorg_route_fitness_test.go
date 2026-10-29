package architecture

// TestNoInOrgNetworkRoute: the storefront calls only the network's Open Host
// Service routes (plus the storefront contexts' own REST and the
// customer-ordering shopper routes). Never an in-org or restricted route:
// ship-to, POST /promises, promise confirm/cancel, promise policies and books,
// inventory feeds. The sensor parses every production file under cmd/ and
// internal/ (except the allowlist itself) and fails on
//   - a string literal that names one of those routes, or
//   - a hard-coded http(s):// URL (upstream hosts come from the environment).
// It also checks the allowlist itself is free of them.
// TestNoInOrgRouteSensorBites proves the detector flags those shapes.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

var restrictedRoute = regexp.MustCompile(`/ship-to|/promises|/promise-policies|/promise-books|/inventory|/stock|/replenish|/allocations|/node-`)

var hardcodedURL = regexp.MustCompile(`^https?://`)

const allowlistFile = "internal/domain/composition/allowlist.go"

func routeLiteralFindings(filename string, src any) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		switch {
		case restrictedRoute.MatchString(s):
			out = append(out, fmt.Sprintf("%s names a restricted route %q", fset.Position(lit.Pos()), s))
		case hardcodedURL.MatchString(s):
			out = append(out, fmt.Sprintf("%s hard-codes the URL %q", fset.Position(lit.Pos()), s))
		}
		return true
	})
	return out, nil
}

func TestNoInOrgNetworkRoute(t *testing.T) {
	var files []string
	for _, dir := range []string{"cmd", "internal"} {
		files = append(files, goFilesUnder(t, filepath.Join("..", "..", dir), false)...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no production files found: the sensor would pass vacuously")
	}
	for _, path := range files {
		slash := filepath.ToSlash(path)
		if strings.HasSuffix(slash, allowlistFile) || strings.Contains(slash, "/internal/architecture/") {
			continue
		}
		findings, err := routeLiteralFindings(path, nil)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, f := range findings {
			t.Errorf("ARCHITECTURE VIOLATION (no in-org network route)\n  WHAT: %s\n  WHY:  the storefront may call only the network's Open Host Service routes; in-org and restricted routes stay behind the network edge.\n  FIX:  call a listed OHS route through composition.Allowed, and take hosts from the environment. Do NOT loosen this test.", f)
		}
	}
}

func TestAllowlistHoldsNoRestrictedRoute(t *testing.T) {
	total := 0
	for svc, routes := range composition.AllowedRoutes() {
		for _, r := range routes {
			total++
			if restrictedRoute.MatchString(r) {
				t.Errorf("%s allowlist names a restricted route: %s", svc, r)
			}
			if !strings.HasPrefix(r, "GET /") && !strings.HasPrefix(r, "POST /") {
				t.Errorf("%s: unexpected method in %q", svc, r)
			}
		}
	}
	if total == 0 {
		t.Fatal("empty allowlist")
	}
}

func TestNoInOrgRouteSensorBites(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{`package p; var a = "/orders/" + "/ship-to"`, 1},
		{`package p; var a = "/promises"`, 1},
		{`package p; var a = "/promises/p1:confirm"`, 1},
		{`package p; var a = "/promise-policies/current"`, 1},
		{`package p; var a = "/inventory/feeds"`, 1},
		{`package p; var a = "http://delivery-promise:8080"`, 1},
		{`package p; var a = "https://example.test/x"`, 1},
		{`package p; var a = "/orders/"; var b = "/tracking"; var c = "/delivery-offers:quote"`, 0},
		{"package p; var a = `/carts/x`", 0},
	}
	for _, tc := range cases {
		got, err := routeLiteralFindings("src.go", tc.src)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.src, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s: findings = %v, want %d", tc.src, got, tc.want)
		}
	}
}
