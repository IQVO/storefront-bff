package http

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// maxBodyBytes caps a request body (a registration with 20 addresses fits
// many times over).
const maxBodyBytes = 1 << 20

// rejectNUL turns a NUL character in the path, query or JSON body into a
// client error before it can reach an upstream's Postgres text parameter.
func rejectNUL(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasNUL(r.URL.Path) || hasNUL(r.URL.RawPath) {
			writeProblem(w, pNotFound, "no resource matches the requested path")
			return
		}
		if hasNUL(r.URL.RawQuery) {
			writeProblem(w, pMalformed, "a NUL character is not allowed in the query string")
			return
		}
		if r.Body != nil && r.Body != http.NoBody {
			body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
			_ = r.Body.Close()
			switch {
			case err != nil:
				writeProblem(w, pMalformed, "the request body could not be read")
				return
			case len(body) > maxBodyBytes:
				writeProblem(w, pTooLarge, "")
				return
			case hasNUL(string(body)) || bytes.Contains(body, []byte(`\u0000`)):
				writeProblem(w, pMalformed, "a NUL character is not allowed in a request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		next.ServeHTTP(w, r)
	})
}

func hasNUL(s string) bool { return strings.ContainsRune(s, 0) || strings.Contains(s, "%00") }

// recoverPanic answers 500 instead of dropping the connection. The panic
// value is not logged: it could embed request data.
func recoverPanic(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recover() != nil {
					log.Error("handler panic", "method", r.Method, "path", r.URL.Path)
					writeProblem(w, pInternal, "")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
