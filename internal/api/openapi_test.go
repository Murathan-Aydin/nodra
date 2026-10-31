package api

import (
	"strings"
	"testing"
)

func TestOpenAPI(t *testing.T) {
	e := newTestEnv(t)
	c := newClient(t, e.srv)
	var spec struct {
		Paths map[string]map[string]struct {
			Summary string   `json:"summary"`
			Tags    []string `json:"tags"`
		} `json:"paths"`
	}
	expect(t, "spécification", c.raw("GET", e.srv.URL+"/api/openapi.json", nil, &spec), 200)
	db := spec.Paths["/api/v1/teams/{team}/databases"]["post"]
	if !strings.Contains(db.Summary, "Database") || len(db.Tags) != 1 || db.Tags[0] != "Bases de données" {
		t.Fatalf("POST /databases : %+v", db)
	}
	ops, documented := 0, 0
	for path, methods := range spec.Paths {
		for m, op := range methods {
			ops++
			if op.Summary != "" {
				documented++
			} else if testing.Verbose() {
				t.Log("non documentée :", m, path)
			}
		}
	}
	if ops < 150 || documented*100/ops < 85 {
		t.Fatalf("couverture : %d opérations, %d documentées", ops, documented)
	}
	t.Logf("%d opérations, %d documentées", ops, documented)
	expect(t, "swagger", c.raw("GET", e.srv.URL+"/api/docs", nil, nil), 200)
}
