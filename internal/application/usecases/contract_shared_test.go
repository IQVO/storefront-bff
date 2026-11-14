// Consumer contract tests against the byte-pinned producer specs: shared
// helpers. testdata/*.openapi.yaml are byte copies of each producer develop's
// apis/openapi.yaml; contracts/pinned.json and the pinned-contracts CI job
// keep them equal to the producers. The copies buy four guarantees per
// service: R1 the routes the BFF calls are declared, R2 the example fixtures
// satisfy the producer's schemas, R3 the REAL rest client and gateway decode
// the producer's shapes (problem+json passes through untouched, transport
// failures become the composition sentinels), R4 every wire field the BFF
// decodes is specified with a compatible type, and fields the schema does
// NOT require are ones the decoding tolerates missing.
package usecases

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/IQVO/storefront-bff/internal/adapters/outbound/rest"
	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// loadPinnedSpec loads one pinned producer spec copy.
func loadPinnedSpec(t *testing.T, file string) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("testdata/" + file)
	if err != nil {
		t.Fatalf("load pinned spec %s: %v", file, err)
	}
	return doc
}

// readContractExample reads a fixture from testdata.
func readContractExample(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// checkRouteDeclared is R1 for one call site: the pinned spec must declare
// the (method, path template) the BFF calls.
func checkRouteDeclared(t *testing.T, doc *openapi3.T, method, path string) {
	t.Helper()
	item := doc.Paths.Find(path)
	if item == nil {
		t.Errorf("pinned spec has no path %s, the BFF calls it", path)
		return
	}
	if item.Operations()[method] == nil {
		t.Errorf("pinned spec path %s has no %s operation", path, method)
	}
}

// checkExampleSatisfiesSchema is R2 for one fixture: the example must satisfy
// the pinned schema of the message it stands for.
func checkExampleSatisfiesSchema(t *testing.T, doc *openapi3.T, fixture, schema string) {
	t.Helper()
	var v any
	if err := json.Unmarshal(readContractExample(t, fixture), &v); err != nil {
		t.Errorf("%s: %v", fixture, err)
		return
	}
	ref := doc.Components.Schemas[schema]
	if ref == nil || ref.Value == nil {
		t.Errorf("%s: schema %s is not in the pinned spec copy", fixture, schema)
		return
	}
	if err := ref.Value.VisitJSON(v); err != nil {
		t.Errorf("%s does not satisfy the pinned %s schema: %v", fixture, schema, err)
	}
}

// checkRequestBodySchema is R2 for a request body the BFF builds: it returns
// the operation's application/json schema for validating what was sent.
func requestBodySchema(t *testing.T, doc *openapi3.T, method, path string) *openapi3.Schema {
	t.Helper()
	item := doc.Paths.Find(path)
	if item == nil || item.Operations()[method] == nil {
		t.Fatalf("%s %s is not in the pinned spec copy", method, path)
	}
	body := item.Operations()[method].RequestBody
	if body == nil || body.Value == nil {
		t.Fatalf("%s %s declares no request body", method, path)
	}
	schema := body.Value.Content["application/json"].Schema
	if schema == nil || schema.Value == nil {
		t.Fatalf("%s %s declares no application/json body schema", method, path)
	}
	return schema.Value
}

// checkBodySatisfiesSchema validates a request the BFF actually sent (R3)
// against the producer's request schema.
func checkBodySatisfiesSchema(t *testing.T, schema *openapi3.Schema, body []byte, what string) {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Errorf("%s is not JSON: %v", what, err)
		return
	}
	if err := schema.VisitJSON(v); err != nil {
		t.Errorf("%s does not satisfy the producer's request schema: %v", what, err)
	}
}

// loggedCall is one upstream request the REAL client made.
type loggedCall struct {
	Service, Method, Path, RawQuery, Body, IdempotencyKey, ContentType string
}

// upstreamLog records what the real client sent, per service.
type upstreamLog struct {
	mu    sync.Mutex
	calls []loggedCall
}

func (l *upstreamLog) record(c loggedCall) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, c)
}

// to returns the calls made to one service.
func (l *upstreamLog) to(service composition.Service) []loggedCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []loggedCall
	for _, c := range l.calls {
		if c.Service == string(service) {
			out = append(out, c)
		}
	}
	return out
}

// newUpstreamGateway wires the REAL rest client and the REAL gateway over one
// httptest server that stands in for all six producers (each behind its own
// path prefix). respond answers every call; a nil answer is a 503.
func newUpstreamGateway(
	t *testing.T,
	budgets composition.Budgets,
	respond func(loggedCall) composition.Reply,
) (*Gateway, *upstreamLog) {
	t.Helper()
	log := &upstreamLog{}
	mux := http.NewServeMux()
	for _, s := range composition.Services() {
		sv := s
		prefix := "/" + string(sv)
		mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			c := loggedCall{
				Service: string(sv), Method: r.Method,
				Path: strings.TrimPrefix(r.URL.Path, prefix), RawQuery: r.URL.RawQuery,
				Body: string(body), IdempotencyKey: r.Header.Get("Idempotency-Key"),
				ContentType: r.Header.Get("Content-Type"),
			}
			log.record(c)
			reply := composition.Reply{Status: http.StatusServiceUnavailable, ContentType: "application/problem+json", Body: []byte(`{"type":"/problems/upstream-unavailable"}`)}
			if respond != nil {
				reply = respond(c)
			}
			if reply.Replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			w.Header().Set("Content-Type", reply.ContentType)
			w.WriteHeader(reply.Status)
			_, _ = w.Write(reply.Body)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	bases := make(map[composition.Service]string, len(composition.Services()))
	for _, s := range composition.Services() {
		bases[s] = srv.URL + "/" + string(s)
	}
	client, err := rest.New(bases)
	if err != nil {
		t.Fatalf("rest.New: %v", err)
	}
	return NewGateway(client, budgets), log
}

// jsonFixtureReply answers with a fixture read from testdata.
func jsonFixtureReply(t *testing.T, status int, fixture string) composition.Reply {
	t.Helper()
	return composition.Reply{Status: status, ContentType: "application/json", Body: readContractExample(t, fixture)}
}

// R4 helpers: reflection of the DTOs the BFF decodes against the pinned
// schemas (copied across the fleet's consumer contract tests).

var timeType = reflect.TypeOf(time.Time{})

var rawMessageType = reflect.TypeOf(json.RawMessage{})

func checkDTOFields(t *testing.T, dt reflect.Type, sch *openapi3.Schema, path string, decode bool) {
	t.Helper()
	props, required := flattenSchema(sch)
	for i := range dt.NumField() {
		f := dt.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue // not a wire field
		}
		p := props[name]
		if p == nil || p.Value == nil {
			t.Errorf("%s.%s: json %q is not declared by the pinned schema", path, f.Name, name)
			continue
		}
		checkWireType(t, f.Type, p.Value, path+"."+name, decode)
		if decode && !required[name] && !toleratesAbsence(f) {
			t.Errorf("%s.%s: optional in the pinned schema, so the decoding must tolerate its absence (pointer, slice or omitempty)", path, f.Name)
		}
	}
}

func flattenSchema(sch *openapi3.Schema) (map[string]*openapi3.SchemaRef, map[string]bool) {
	props := map[string]*openapi3.SchemaRef{}
	required := map[string]bool{}
	var walk func(*openapi3.Schema)
	walk = func(s *openapi3.Schema) {
		if s == nil {
			return
		}
		for n, p := range s.Properties {
			props[n] = p
		}
		for _, r := range s.Required {
			required[r] = true
		}
		for _, b := range s.AllOf {
			walk(b.Value)
		}
	}
	walk(sch)
	return props, required
}

func toleratesAbsence(f reflect.StructField) bool {
	_, _, has := strings.Cut(f.Tag.Get("json"), ",omitempty")
	return has || f.Type.Kind() == reflect.Pointer || f.Type.Kind() == reflect.Slice
}

// scalarWireTypes maps the scalar Go kinds a DTO may carry to the OpenAPI
// type that must declare them.
var scalarWireTypes = map[reflect.Kind]string{
	reflect.String: "string", reflect.Bool: "boolean",
	reflect.Int: "integer", reflect.Int8: "integer", reflect.Int16: "integer", reflect.Int32: "integer", reflect.Int64: "integer",
	reflect.Uint: "integer", reflect.Uint8: "integer", reflect.Uint16: "integer", reflect.Uint32: "integer", reflect.Uint64: "integer",
	reflect.Float32: "number", reflect.Float64: "number",
}

func checkWireType(t *testing.T, dt reflect.Type, sch *openapi3.Schema, path string, decode bool) {
	t.Helper()
	if dt == rawMessageType {
		return // an opaque value relayed unread; the fixture validates its shape
	}
	if dt == timeType {
		if !sch.Type.Includes("string") || sch.Format != "date-time" {
			t.Errorf("%s: time.Time, but the pinned schema says %v/%q", path, sch.Type.Slice(), sch.Format)
		}
		return
	}
	switch dt.Kind() {
	case reflect.Pointer:
		checkWireType(t, dt.Elem(), sch, path, decode)
	case reflect.Slice:
		checkArrayWireType(t, dt.Elem(), sch, path, decode)
	case reflect.Struct:
		checkObjectWireType(t, dt, sch, path, decode)
	default:
		if want, scalar := scalarWireTypes[dt.Kind()]; scalar {
			assertWireType(t, sch, path, want)
		} else {
			t.Errorf("%s: Go kind %s has no wire mapping", path, dt.Kind())
		}
	}
}

func checkArrayWireType(t *testing.T, elem reflect.Type, sch *openapi3.Schema, path string, decode bool) {
	t.Helper()
	if assertWireType(t, sch, path, "array") && sch.Items != nil && sch.Items.Value != nil {
		checkWireType(t, elem, sch.Items.Value, path+"[]", decode)
	}
}

func checkObjectWireType(t *testing.T, dt reflect.Type, sch *openapi3.Schema, path string, decode bool) {
	t.Helper()
	if assertWireType(t, sch, path, "object") {
		checkDTOFields(t, dt, sch, path, decode)
	}
}

func assertWireType(t *testing.T, sch *openapi3.Schema, path, want string) bool {
	t.Helper()
	if len(sch.Type.Slice()) > 0 && !sch.Type.Includes(want) {
		t.Errorf("%s: pinned schema type %v, the Go field is %s", path, sch.Type.Slice(), want)
		return false
	}
	return true
}
