// Package version : version de Nodra, fixée à la compilation
// (go build -ldflags "-X github.com/Murathan-Aydin/nodra/internal/version.Version=v1.2.3").
package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Version de ce binaire ("dev" pour une compilation locale).
var Version = "dev"

// Repository : dépôt GitHub dont les releases annoncent les nouvelles versions.
const Repository = "Murathan-Aydin/nodra"

// Newer indique si latest (ex. v1.3.0) est plus récente que current. Une version de développement
// ou illisible n'est jamais considérée comme en retard.
func Newer(latest, current string) bool {
	l, ok1 := parse(latest)
	c, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range 3 {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-") // pré-versions ignorées
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// ReleasesAPI : dernière release publiée du dépôt.
const ReleasesAPI = "https://api.github.com/repos/" + Repository + "/releases/latest"

// Latest renvoie l'étiquette de la dernière release ("" si aucune n'est publiée).
func Latest(ctx context.Context, api string) (string, error) {
	if api == "" {
		api = ReleasesAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub a répondu HTTP %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}
