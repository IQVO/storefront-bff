package bdd

import (
	"github.com/cucumber/godog"
)

// steps_wave2_test.go — the wave-2 additions: the query a relayed search
// carried upstream, and the number of upstream calls a journey made.
func registerWave2(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^the last upstream call was "([A-Z]+) ([^"]+)" carrying the query "([^"]*)"$`, func(method, path, query string) error {
		c, err := w.lastCall()
		if err != nil {
			return err
		}
		return expect(c.Method == method && c.Path == path && c.Query.Encode() == query,
			"upstream call = %+v (query %q)", c, c.Query.Encode())
	})
	sc.Step(`^(\d+) upstream calls? were made$`, func(n int) error {
		return expect(len(w.up.calls) == n, "upstream calls = %d, want %d", len(w.up.calls), n)
	})
}
