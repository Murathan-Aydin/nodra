package envvars

import (
	"reflect"
	"testing"
)

func TestParseDotenv(t *testing.T) {
	in := `# commentaire
export DB_HOST=localhost
PORT = 3000 # commentaire en fin de ligne
EMPTY=
QUOTED="valeur avec # dièse et \"guillemets\""
SINGLE='pas d interpolation $HOME'
MULTI="ligne 1
ligne 2"
ESC="a\nb"
`
	got, err := ParseDotenv(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []Pair{
		{"DB_HOST", "localhost", false},
		{"PORT", "3000", false},
		{"EMPTY", "", false},
		{"QUOTED", `valeur avec # dièse et "guillemets"`, false},
		{"SINGLE", "pas d interpolation $HOME", false},
		{"MULTI", "ligne 1\nligne 2", true},
		{"ESC", "a\nb", false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\nobtenu %#v\nattendu %#v", got, want)
	}
	if _, err := ParseDotenv("1BAD=x"); err == nil {
		t.Fatal("clé invalide acceptée")
	}
	if _, err := ParseDotenv(`A="non fermé`); err == nil {
		t.Fatal("guillemet non fermé accepté")
	}
}

func TestInterpolate(t *testing.T) {
	shared := map[string]string{"team.DB_HOST": "db", "environment.PORT": "5432"}
	got, missing := Interpolate("postgres://{{team.DB_HOST}}:{{ environment.PORT }}/{{project.NAME}}", shared)
	if got != "postgres://db:5432/" || len(missing) != 1 || missing[0] != "project.NAME" {
		t.Fatalf("%q %v", got, missing)
	}
	if EscapeCompose("a$b$$") != "a$$b$$$$" {
		t.Fatal("échappement compose")
	}
}
