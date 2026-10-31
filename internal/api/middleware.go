package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/auth"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const sessionCookie = "nodra_session"

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSession
	ctxToken
)

// Principal : l'utilisateur authentifié et le moyen utilisé (session ou jeton API).
func userFrom(ctx context.Context) *store.User {
	u, _ := ctx.Value(ctxUser).(*store.User)
	return u
}

func sessionFrom(ctx context.Context) *store.Session {
	s, _ := ctx.Value(ctxSession).(*store.Session)
	return s
}

func tokenFrom(ctx context.Context) *store.APIToken {
	t, _ := ctx.Value(ctxToken).(*store.APIToken)
	return t
}

// authenticate résout la session (cookie) ou le jeton API (Bearer) sans rien exiger.
// Une session en attente de 2FA n'authentifie pas : elle est seulement exposée via ctxSession.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			tok, err := s.Store.APITokenByHash(ctx, auth.HashToken(strings.TrimPrefix(h, "Bearer ")))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "jeton API invalide ou expiré")
				return
			}
			u, err := s.Store.UserByID(ctx, tok.UserID)
			if err != nil || u.DisabledAt != nil {
				writeError(w, http.StatusUnauthorized, "jeton API invalide")
				return
			}
			if inst, err := s.instance(ctx); err == nil {
				if !inst.APIEnabled {
					writeError(w, http.StatusForbidden, "API désactivée par l'administrateur de l'instance")
					return
				}
				if len(inst.APIAllowedIPs) > 0 && !ipAllowed(clientIP(r), inst.APIAllowedIPs) {
					writeError(w, http.StatusForbidden, "adresse IP non autorisée pour l'API")
					return
				}
			}
			ctx = context.WithValue(context.WithValue(ctx, ctxUser, u), ctxToken, tok)
		} else if c, err := r.Cookie(sessionCookie); err == nil {
			if sess, err := s.Store.SessionByHash(ctx, auth.HashToken(c.Value)); err == nil {
				ctx = context.WithValue(ctx, ctxSession, sess)
				if !sess.MFAPending {
					if u, err := s.Store.UserByID(ctx, sess.UserID); err == nil && u.DisabledAt == nil {
						ctx = context.WithValue(ctx, ctxUser, u)
					}
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()) == nil {
			writeError(w, http.StatusUnauthorized, "authentification requise")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireSession refuse les jetons API (gestion du compte réservée à une session interactive).
func requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tokenFrom(r.Context()) != nil {
			writeError(w, http.StatusForbidden, "action impossible avec un jeton API")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfGuard : les requêtes modifiantes authentifiées par cookie doivent être en JSON
// (un formulaire cross-site ne peut pas envoyer application/json sans preflight CORS).
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			// Un corps non-JSON (ex. import d'un dump) n'est accepté qu'avec X-Requested-With, en-tête qu'un site
			// tiers ne peut pas envoyer sans pré-vérification CORS.
			if r.Header.Get("Authorization") == "" && r.ContentLength != 0 &&
				!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") && r.Header.Get("X-Requested-With") == "" {
				writeError(w, http.StatusUnsupportedMediaType, "Content-Type application/json requis")
				return
			}
			if r.Header.Get("Authorization") == "" && r.ContentLength == 0 && r.Header.Get("X-Requested-With") == "" {
				writeError(w, http.StatusForbidden, "en-tête X-Requested-With requis")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimiter : limite simple en mémoire par clé (IP) sur une fenêtre glissante.
type rateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	h := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			h = append(h, t)
		}
	}
	if len(h) >= l.max {
		l.hits[key] = h
		return false
	}
	l.hits[key] = append(h, now)
	return true
}

func (l *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "trop de tentatives, réessayez plus tard")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
