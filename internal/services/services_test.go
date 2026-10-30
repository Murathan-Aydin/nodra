package services

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseMagic(t *testing.T) {
	cases := map[string]MagicVar{
		"SERVICE_PASSWORD_64_PG":     {Kind: "PASSWORD_64", ID: "PG"},
		"SERVICE_PASSWORD_PG":        {Kind: "PASSWORD", ID: "PG"},
		"SERVICE_URL_N8N_5678":       {Kind: "URL", ID: "N8N", Port: "5678"},
		"SERVICE_FQDN_APP_SERVER":    {Kind: "FQDN", ID: "APP_SERVER"},
		"SERVICE_REALBASE64_32_TOTP": {Kind: "REALBASE64_32", ID: "TOTP"},
		"SERVICE_SUPABASEANON_KEY":   {Kind: "SUPABASEANON", ID: "KEY"},
	}
	for name, want := range cases {
		got, ok := ParseMagic(name)
		if !ok || got.Kind != want.Kind || got.ID != want.ID || got.Port != want.Port {
			t.Errorf("%s : %+v", name, got)
		}
	}
	for _, name := range []string{"POSTGRES_DB", "SERVICE_UNKNOWN_X", "SERVICE_PASSWORD_"} {
		if _, ok := ParseMagic(name); ok {
			t.Errorf("%s ne doit pas être magique", name)
		}
	}
}

func TestAnalyze(t *testing.T) {
	raw := `services:
  my-app:
    image: ghcr.io/x/app:1
    environment:
      - SERVICE_URL_MYAPP_3000
      - DATABASE_URL=postgres://${SERVICE_USER_PG}:${SERVICE_PASSWORD_PG}@db/${POSTGRES_DB:-app}
      - SECRET=${SERVICE_BASE64_64_APP}
      - SMTP_HOST=${SMTP_HOST}
      - API=${API_KEY:?requis}
      - SITE=${SITE_URL:-${SERVICE_URL_SITE}}
  db:
    image: postgres:16
    environment:
      POSTGRES_USER: ${SERVICE_USER_PG}
      POSTGRES_PASSWORD: ${SERVICE_PASSWORD_PG}
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $${POSTGRES_USER}"]
  admin:
    image: adminer
    environment:
      - LINK=${SERVICE_URL_ADMIN}
`
	a, err := Analyze(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(a.Services, ",") != "admin,db,my-app" || a.Images["db"] != "postgres:16" {
		t.Fatalf("services : %v %v", a.Services, a.Images)
	}
	if e := a.Exposed["my-app"]; e.ID != "MYAPP" || e.Port != "3000" {
		t.Fatalf("my-app exposé par SERVICE_URL_MYAPP_3000 : %+v", a.Exposed)
	}
	if e := a.Exposed["admin"]; e.ID != "ADMIN" {
		t.Fatalf("admin exposé par correspondance de nom : %+v", a.Exposed)
	}
	for _, name := range []string{"SERVICE_USER_PG", "SERVICE_PASSWORD_PG", "SERVICE_BASE64_64_APP"} {
		if _, ok := a.Magic[name]; !ok {
			t.Errorf("variable magique manquante : %s", name)
		}
	}
	users := map[string]UserVar{}
	for _, u := range a.User {
		users[u.Key] = u
	}
	if _, ok := a.Magic["SERVICE_URL_SITE"]; !ok || users["SITE_URL"].Default != "" {
		t.Fatalf("variable magique imbriquée : %+v %+v", a.Magic, users["SITE_URL"])
	}
	if users["POSTGRES_DB"].Default != "app" || !users["API_KEY"].Required || len(users) != 4 {
		t.Fatalf("variables configurables : %+v", a.User)
	}
	if _, ok := users["POSTGRES_USER"]; ok {
		t.Fatal("$${POSTGRES_USER} est une variable du conteneur")
	}
	if _, err := Analyze("services: ["); err == nil {
		t.Fatal("YAML invalide accepté")
	}
}

func TestGenerate(t *testing.T) {
	existing := map[string]string{}
	for _, m := range []MagicVar{{Kind: "PASSWORD"}, {Kind: "PASSWORD_64"}, {Kind: "HEX_32"}, {Kind: "REALBASE64_32"}, {Kind: "USER"}} {
		v, ok := Generate(m, existing)
		if !ok || v == "" {
			t.Fatalf("%s non généré", m.Kind)
		}
		switch m.Kind {
		case "PASSWORD_64":
			if len(v) != 64 {
				t.Errorf("longueur %d", len(v))
			}
		case "HEX_32":
			if len(v) != 32 {
				t.Errorf("hex %q", v)
			}
		case "REALBASE64_32":
			if b, err := base64.StdEncoding.DecodeString(v); err != nil || len(b) != 32 {
				t.Errorf("base64 %q", v)
			}
		}
	}
	if _, ok := Generate(MagicVar{Kind: "SUPABASEANON"}, existing); ok {
		t.Fatal("JWT impossible sans SERVICE_PASSWORD_JWT")
	}
	existing["SERVICE_PASSWORD_JWT"] = "secret"
	jwt, ok := Generate(MagicVar{Kind: "SUPABASEANON"}, existing)
	if !ok || strings.Count(jwt, ".") != 2 {
		t.Fatalf("JWT : %q", jwt)
	}
	claims, _ := base64.RawURLEncoding.DecodeString(strings.Split(jwt, ".")[1])
	if !strings.Contains(string(claims), `"role":"anon"`) {
		t.Fatalf("revendications : %s", claims)
	}
}

func TestCatalog(t *testing.T) {
	all := Catalog("", "")
	if len(all) < 300 {
		t.Fatalf("catalogue : %d modèles", len(all))
	}
	n8n, err := Get("n8n")
	if err != nil || n8n.Category == "" || !strings.Contains(n8n.Compose, "services:") || strings.HasPrefix(n8n.Compose, "#") {
		t.Fatalf("n8n : %+v %v", n8n, err)
	}
	if got := Catalog("workflow", ""); len(got) == 0 || got[0].Compose != "" {
		t.Fatal("recherche sans contenu compose")
	}
	if _, err := Get("plausible"); err == nil {
		t.Fatal("les modèles « ignore » ne sont pas proposés")
	}
	// Tous les modèles proposés doivent être analysables.
	for _, tpl := range all {
		full, _ := Get(tpl.Slug)
		if _, err := Analyze(full.Compose); err != nil {
			t.Errorf("%s : %v", tpl.Slug, err)
		}
	}
	if len(Categories()) < 5 {
		t.Fatal("catégories")
	}
}
