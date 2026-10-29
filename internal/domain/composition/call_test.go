package composition

import "testing"

func TestReplyOK(t *testing.T) {
	cases := map[int]bool{199: false, 200: true, 201: true, 299: true, 300: false, 404: false, 503: false}
	for status, want := range cases {
		if got := (Reply{Status: status}).OK(); got != want {
			t.Errorf("status %d: OK() = %v, want %v", status, got, want)
		}
	}
}

func TestServicesAreSixDistinct(t *testing.T) {
	seen := map[Service]bool{}
	for _, s := range Services() {
		if s == "" || seen[s] {
			t.Fatalf("service %q empty or duplicated", s)
		}
		seen[s] = true
	}
	if len(seen) != 6 {
		t.Fatalf("got %d services, want 6", len(seen))
	}
}
