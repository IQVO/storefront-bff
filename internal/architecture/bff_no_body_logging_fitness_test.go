package architecture

// TestBFFNeverLogsBodies: the storefront edge relays carts, checkout tokens,
// e-mails and saved addresses. None of it may reach a log line. The sensor
// parses every production file under cmd/ and internal/ and fails when
//   - a logging call (slog / log / a *log* receiver) has an argument that names
//     a body or payload (body, Body, reply.Body, payload ...), or
//   - the file imports net/http/httputil (DumpRequest / DumpResponse).
// TestBFFBodyLogSensorBites proves the detector flags those shapes.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var logMethods = map[string]bool{
	"Debug": true, "Info": true, "Warn": true, "Error": true, "Log": true, "LogAttrs": true,
	"Print": true, "Printf": true, "Println": true, "Fatal": true, "Fatalf": true, "Panic": true, "Panicf": true,
}

func isLogReceiver(x ast.Expr) bool {
	name := ""
	switch v := x.(type) {
	case *ast.Ident:
		name = v.Name
	case *ast.SelectorExpr:
		name = v.Sel.Name
	}
	return strings.Contains(strings.ToLower(name), "log")
}

func namesBody(n ast.Node) (string, bool) {
	var hit string
	ast.Inspect(n, func(c ast.Node) bool {
		var name string
		switch v := c.(type) {
		case *ast.Ident:
			name = v.Name
		case *ast.SelectorExpr:
			name = v.Sel.Name
		default:
			return true
		}
		if l := strings.ToLower(name); strings.Contains(l, "body") || strings.Contains(l, "payload") {
			hit = name
		}
		return hit == ""
	})
	return hit, hit != ""
}

// bodyLogFindings parses src and returns one finding per violation.
func bodyLogFindings(filename string, src any) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p == "net/http/httputil" {
			out = append(out, fmt.Sprintf("%s imports net/http/httputil", fset.Position(imp.Pos())))
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !logMethods[sel.Sel.Name] || !isLogReceiver(sel.X) {
			return true
		}
		for _, arg := range call.Args {
			if name, hit := namesBody(arg); hit {
				out = append(out, fmt.Sprintf("%s logs %q", fset.Position(call.Pos()), name))
			}
		}
		return true
	})
	return out, nil
}

func TestBFFNeverLogsBodies(t *testing.T) {
	var files []string
	for _, dir := range []string{"cmd", "internal"} {
		files = append(files, goFilesUnder(t, filepath.Join("..", "..", dir), false)...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no production files found: the sensor would pass vacuously")
	}
	for _, path := range files {
		if strings.Contains(filepath.ToSlash(path), "/internal/architecture/") {
			continue
		}
		findings, err := bodyLogFindings(path, nil)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, f := range findings {
			t.Errorf("ARCHITECTURE VIOLATION (BFF never logs bodies)\n  WHAT: %s\n  WHY:  request and response bodies carry e-mails, saved addresses and payment tokens.\n  FIX:  log method, route pattern, status and duration only. Do NOT loosen this test.", f)
		}
	}
}

func TestBFFBodyLogSensorBites(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"slog with body arg", "package p\nfunc f(log L, body []byte){ log.Info(\"x\", \"body\", string(body)) }", 1},
		{"field Body", "package p\nfunc f(s S, reply R){ s.log.Error(\"x\", \"b\", reply.Body) }", 1},
		{"payload via log.Printf", "package p\nfunc f(payload string){ log.Printf(\"%s\", payload) }", 1},
		{"Debug of request body", "package p\nfunc f(logger L, r R){ logger.Debug(\"x\", r.Body) }", 1},
		{"httputil import", "package p\nimport \"net/http/httputil\"\nvar _ = httputil.DumpRequest", 1},
		{"method route status only", "package p\nfunc f(log L, r R){ log.Info(\"request\", \"method\", r.Method, \"status\", 200) }", 0},
		{"body used outside logging", "package p\nfunc f(w W, body []byte){ w.Write(body) }", 0},
		{"non-log receiver", "package p\nfunc f(c C, body []byte){ c.Info(body) }", 0},
	}
	for _, tc := range cases {
		got, err := bodyLogFindings("src.go", tc.src)
		if err != nil {
			t.Fatalf("%s: parse: %v", tc.name, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s: findings = %v, want %d", tc.name, got, tc.want)
		}
	}
}
