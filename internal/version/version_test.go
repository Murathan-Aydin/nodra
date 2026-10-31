package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.0", "v1.1.9", true},
		{"v1.10.0", "v1.9.0", true},
		{"1.2.0", "v1.2.0", false},
		{"v1.2.0", "v1.3.0", false},
		{"v2.0.0-rc.1", "v1.9.9", true},
		{"v1.2.0", "dev", false},
		{"", "v1.0.0", false},
	} {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/none" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name": "v1.4.2"}`))
	}))
	defer srv.Close()
	if v, err := Latest(context.Background(), srv.URL+"/latest"); err != nil || v != "v1.4.2" {
		t.Fatalf("%q %v", v, err)
	}
	if v, err := Latest(context.Background(), srv.URL+"/none"); err != nil || v != "" {
		t.Fatalf("sans release : %q %v", v, err)
	}
}
