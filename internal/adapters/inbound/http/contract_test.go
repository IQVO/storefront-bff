package http

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// TestRoutesMatchOpenAPI keeps apis/openapi.yaml and the router in step: every
// documented operation is routed and every routed operation is documented.
func TestRoutesMatchOpenAPI(t *testing.T) {
	raw, err := os.ReadFile("../../../../apis/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	var documented []string
	for path, ops := range spec.Paths {
		for method := range ops {
			documented = append(documented, strings.ToUpper(method)+" "+path)
		}
	}
	var routed []string
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if route != "/healthz" && route != "/readyz" {
			routed = append(routed, method+" "+route)
		}
		return nil
	}
	if err := chi.Walk(newRig().server.router(), walk); err != nil {
		t.Fatal(err)
	}
	sort.Strings(documented)
	sort.Strings(routed)
	if strings.Join(documented, "\n") != strings.Join(routed, "\n") {
		t.Fatalf("OpenAPI and router differ.\nopenapi:\n%s\nrouter:\n%s", strings.Join(documented, "\n"), strings.Join(routed, "\n"))
	}
}
