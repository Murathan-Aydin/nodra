package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/Murathan-Aydin/nodra/internal/git"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const maxWebhookBody = 5 << 20

// githubAppWebhook reçoit les événements d'une GitHub App (secret propre à l'App).
func (s *Server) githubAppWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "corps trop volumineux")
		return
	}
	appID, _ := strconv.ParseInt(r.Header.Get("X-GitHub-Hook-Installation-Target-ID"), 10, 64)
	sources, err := s.Store.GitSourcesByAppID(r.Context(), appID)
	if err != nil {
		s.internal(w, err)
		return
	}
	var src *store.GitSource
	for _, g := range sources {
		secret, err := s.Box.Decrypt(g.WebhookSecretEnc)
		if err == nil && git.Verify(git.GitHub, secret, r.Header, body) == nil {
			src = g
			break
		}
	}
	if src == nil {
		writeError(w, http.StatusUnauthorized, "signature invalide ou App inconnue")
		return
	}
	ev, err := git.Parse(git.GitHub, r.Header, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch ev.Kind {
	case "ping":
		writeJSON(w, http.StatusOK, map[string]string{"message": "pong"})
		return
	case "installation":
		switch ev.Action {
		case "created":
			src.InstallationID = ev.InstallationID
		case "deleted", "suspend":
			src.InstallationID = 0
		}
		if err := s.Store.UpdateGitSource(r.Context(), src); err != nil {
			s.internal(w, err)
			return
		}
		git.ForgetToken(src.ID)
		writeJSON(w, http.StatusOK, map[string]string{"message": "installation " + ev.Action})
		return
	case "push", "pull_request":
		if s.GitEvents == nil {
			writeJSON(w, http.StatusAccepted, map[string]string{"message": "aucune application à déployer"})
			return
		}
		res, err := s.GitEvents.HandleGitEvent(r.Context(), src, ev)
		if err != nil {
			s.internal(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "événement ignoré"})
}
