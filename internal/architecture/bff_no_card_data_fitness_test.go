package architecture

// TestNoCardDataAnywhere: the storefront never sees a card number, only PSP
// tokens. The sensor fails when, anywhere in production Go code or in the
// BFF's own OpenAPI contract,
//   - a struct field or JSON tag is named like raw card data (pan, cvc, cvv,
//     cardNumber, expiry ...), or
//   - a string literal is a 13-19 digit Luhn-valid number (a PAN), or
//   - the OpenAPI declares a property with such a name.
// TestNoCardDataSensorBites proves the detector flags those shapes.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var cardNames = regexp.MustCompile(`(?i)^(pan|cvc2?|cvv2?|cardnumber|card_number|securitycode|security_code|expiry|expirymonth|expiryyear|expirydate|cardholder|cardholdername)$`)

func luhnPAN(s string) bool {
	if len(s) < 13 || len(s) > 19 {
		return false
	}
	sum := 0
	for i := 0; i < len(s); i++ {
		c := s[len(s)-1-i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i%2 == 1 {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

func fieldFindings(fset *token.FileSet, v *ast.Field) []string {
	var out []string
	for _, name := range v.Names {
		if cardNames.MatchString(name.Name) {
			out = append(out, fmt.Sprintf("%s field %q looks like card data", fset.Position(name.Pos()), name.Name))
		}
	}
	if v.Tag == nil {
		return out
	}
	tag, _ := strconv.Unquote(v.Tag.Value)
	for _, part := range strings.Split(tag, `"`) {
		if name, _, _ := strings.Cut(part, ","); cardNames.MatchString(name) {
			out = append(out, fmt.Sprintf("%s tag %q looks like card data", fset.Position(v.Tag.Pos()), name))
		}
	}
	return out
}

func literalFindings(fset *token.FileSet, v *ast.BasicLit) []string {
	s, err := strconv.Unquote(v.Value)
	if v.Kind != token.STRING || err != nil {
		return nil
	}
	if luhnPAN(strings.NewReplacer(" ", "", "-", "").Replace(s)) {
		return []string{fmt.Sprintf("%s string literal is a card number", fset.Position(v.Pos()))}
	}
	return nil
}

func cardFindings(filename string, src any) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Field:
			out = append(out, fieldFindings(fset, v)...)
		case *ast.BasicLit:
			out = append(out, literalFindings(fset, v)...)
		}
		return true
	})
	return out, nil
}

var yamlKey = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*):`)

func yamlCardFindings(doc string) []string {
	var out []string
	for i, line := range strings.Split(doc, "\n") {
		if m := yamlKey.FindStringSubmatch(line); m != nil && cardNames.MatchString(m[1]) {
			out = append(out, fmt.Sprintf("openapi line %d declares %q", i+1, m[1]))
		}
	}
	return out
}

func TestNoCardDataAnywhere(t *testing.T) {
	var files []string
	for _, dir := range []string{"cmd", "internal"} {
		files = append(files, goFilesUnder(t, filepath.Join("..", "..", dir), false)...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no production files found: the sensor would pass vacuously")
	}
	var findings []string
	for _, path := range files {
		if strings.Contains(filepath.ToSlash(path), "/internal/architecture/") {
			continue
		}
		got, err := cardFindings(path, nil)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		findings = append(findings, got...)
	}
	if spec, err := os.ReadFile(filepath.Join("..", "..", "apis", "openapi.yaml")); err == nil {
		findings = append(findings, yamlCardFindings(string(spec))...)
	}
	for _, f := range findings {
		t.Errorf("ARCHITECTURE VIOLATION (no card data)\n  WHAT: %s\n  WHY:  the storefront handles PSP payment-method tokens only; a card number or CVC must never be modelled, logged or forwarded.\n  FIX:  use paymentMethodToken. Do NOT loosen this test.", f)
	}
}

func TestNoCardDataSensorBites(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{"package p; type T struct{ PAN string }", 1},
		{"package p; type T struct{ X string `json:\"cvc\"` }", 1},
		{"package p; type T struct{ X string `json:\"cardNumber,omitempty\"` }", 1},
		{"package p; type T struct{ Expiry string }", 1},
		{`package p; var a = "4111111111111111"`, 1},
		{`package p; var a = "4111 1111 1111 1111"`, 1},
		{"package p; type T struct{ PaymentMethodToken string `json:\"paymentMethodToken\"` }", 0},
		{`package p; var a = "1234567890123"`, 0},
		{`package p; var a = "sf-0123456789abcdef"`, 0},
	}
	for _, tc := range cases {
		got, err := cardFindings("src.go", tc.src)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.src, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s: findings = %v, want %d", tc.src, got, tc.want)
		}
	}
	if got := yamlCardFindings("props:\n  paymentMethodToken:\n    type: string\n  cvc:\n    type: string\n"); len(got) != 1 {
		t.Errorf("yaml findings = %v", got)
	}
}
