package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Murathan-Aydin/nodra/internal/auth"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const (
	sessionTTL = 30 * 24 * time.Hour
	mfaTTL     = 10 * time.Minute
	resetTTL   = time.Hour
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	settings, err := s.Store.InstanceSettings(ctx)
	if err != nil {
		s.internal(w, err)
		return
	}
	if n, err := s.Store.CountUsers(ctx); err != nil {
		s.internal(w, err)
		return
	} else if n > 0 && !settings.RegistrationEnabled {
		writeError(w, http.StatusForbidden, "les inscriptions sont fermées sur cette instance")
		return
	}
	email := strings.TrimSpace(in.Email)
	if _, err := mail.ParseAddress(email); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "adresse e-mail invalide")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	u, err := s.Store.CreateUser(ctx, email, strings.TrimSpace(in.Name), hash)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "un compte existe déjà avec cette adresse")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	if err := s.startSession(w, r, u.ID, false); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(withUser(r, u), nil, "auth.registered", "user", u.ID, nil)
	if err := s.sendVerification(r.Context(), u); err != nil {
		s.Log.Warn("e-mail de vérification", "user", u.ID, "err", err)
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Store.UserByEmail(r.Context(), strings.TrimSpace(in.Email))
	if errors.Is(err, store.ErrNotFound) {
		auth.BurnTime(in.Password)
		writeError(w, http.StatusUnauthorized, "identifiants invalides")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	if ok, _ := auth.VerifyPassword(in.Password, u.PasswordHash); !ok {
		s.audit(withUser(r, u), nil, "auth.login.failed", "user", u.ID, nil)
		writeError(w, http.StatusUnauthorized, "identifiants invalides")
		return
	}
	if u.DisabledAt != nil {
		s.audit(withUser(r, u), nil, "auth.login.disabled", "user", u.ID, nil)
		writeError(w, http.StatusForbidden, "compte désactivé par l'administrateur de l'instance")
		return
	}
	mfa := u.TwoFactorEnabled()
	if !mfa {
		s.audit(withUser(r, u), nil, "auth.login", "user", u.ID, nil)
	}
	if err := s.startSession(w, r, u.ID, mfa); err != nil {
		s.internal(w, err)
		return
	}
	if mfa {
		writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mfa_required": false, "user": u})
}

// loginTwoFactor finalise une connexion avec un code TOTP ou un code de secours.
func (s *Server) loginTwoFactor(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	if sess == nil || !sess.MFAPending {
		writeError(w, http.StatusUnauthorized, "aucune connexion en attente de 2FA")
		return
	}
	var in struct {
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	u, err := s.Store.UserByID(ctx, sess.UserID)
	if err != nil {
		s.internal(w, err)
		return
	}
	ok := false
	if in.RecoveryCode != "" {
		ok, err = s.Store.UseRecoveryCode(ctx, u.ID, auth.HashToken(strings.ToLower(strings.TrimSpace(in.RecoveryCode))))
		if err != nil {
			s.internal(w, err)
			return
		}
	} else {
		ok = s.checkTOTP(u, in.Code)
	}
	if !ok {
		s.audit(withUser(r, u), nil, "auth.login.2fa_failed", "user", u.ID, nil)
		writeError(w, http.StatusUnauthorized, "code invalide")
		return
	}
	s.audit(withUser(r, u), nil, "auth.login", "user", u.ID, map[string]any{"two_factor": true, "recovery_code": in.RecoveryCode != ""})
	// Nouvelle session complète (rotation du jeton après élévation).
	_ = s.Store.DeleteSession(ctx, sess.TokenHash)
	if err := s.startSession(w, r, u.ID, false); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if sess := sessionFrom(r.Context()); sess != nil {
		if err := s.Store.DeleteSession(r.Context(), sess.TokenHash); err != nil {
			s.internal(w, err)
			return
		}
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "two_factor_enabled": u.TwoFactorEnabled()})
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	u := userFrom(r.Context())
	if err := s.Store.UpdateUserProfile(r.Context(), u.ID, strings.TrimSpace(in.Name)); err != nil {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !decode(w, r, &in) {
		return
	}
	u := userFrom(r.Context())
	if ok, _ := auth.VerifyPassword(in.Current, u.PasswordHash); !ok {
		writeError(w, http.StatusUnauthorized, "mot de passe actuel incorrect")
		return
	}
	hash, err := auth.HashPassword(in.New)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.Store.SetPassword(r.Context(), u.ID, hash, sessionFrom(r.Context()).TokenHash); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "auth.password.changed", "user", u.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// forgotPassword répond toujours 204 pour ne pas révéler l'existence d'un compte.
// L'envoi d'e-mail arrive avec le module de notifications ; en attendant le lien est journalisé.
func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Store.UserByEmail(r.Context(), strings.TrimSpace(in.Email))
	if err == nil {
		tok := auth.NewToken(auth.PrefixReset)
		if err := s.Store.CreatePasswordReset(r.Context(), u.ID, auth.HashToken(tok), time.Now().Add(resetTTL)); err != nil {
			s.internal(w, err)
			return
		}
		if s.Mailer != nil {
			s.Mailer.PasswordReset(r.Context(), u.Email, tok)
		} else {
			s.Log.Warn("aucun e-mail configuré : jeton de réinitialisation", "email", u.Email, "token", tok)
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	uid, err := s.Store.ConsumePasswordReset(r.Context(), auth.HashToken(in.Token))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "lien de réinitialisation invalide ou expiré")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	if err := s.Store.SetPassword(r.Context(), uid, hash, ""); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "auth.password.reset", "user", uid, nil)
	w.WriteHeader(http.StatusNoContent)
}

// --- 2FA ---

func (s *Server) setupTwoFactor(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if u.TwoFactorEnabled() {
		writeError(w, http.StatusConflict, "la 2FA est déjà activée")
		return
	}
	secret := auth.NewTOTPSecret()
	enc, err := s.Box.Encrypt(secret)
	if err != nil {
		s.internal(w, err)
		return
	}
	if err := s.Store.SetTOTPSecret(r.Context(), u.ID, enc); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "otpauth_url": auth.TOTPURL("Nodra", u.Email, secret)})
}

func (s *Server) enableTwoFactor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	u := userFrom(r.Context())
	if u.TwoFactorEnabled() || u.TOTPSecretEnc == nil {
		writeError(w, http.StatusConflict, "lancez d'abord la configuration de la 2FA")
		return
	}
	if !s.checkTOTP(u, in.Code) {
		writeError(w, http.StatusUnprocessableEntity, "code invalide")
		return
	}
	codes := auth.NewRecoveryCodes(8)
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = auth.HashToken(c)
	}
	if err := s.Store.EnableTOTP(r.Context(), u.ID, hashes); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "auth.2fa.enabled", "user", u.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (s *Server) disableTwoFactor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	u := userFrom(r.Context())
	if ok, _ := auth.VerifyPassword(in.Password, u.PasswordHash); !ok {
		writeError(w, http.StatusUnauthorized, "mot de passe incorrect")
		return
	}
	if err := s.Store.DisableTOTP(r.Context(), u.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "auth.2fa.disabled", "user", u.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) checkTOTP(u *store.User, code string) bool {
	if u.TOTPSecretEnc == nil {
		return false
	}
	secret, err := s.Box.Decrypt(*u.TOTPSecretEnc)
	return err == nil && auth.ValidateTOTP(secret, code, time.Now())
}

// --- Sessions ---

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSessions(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	cur := sessionFrom(r.Context())
	out := make([]map[string]any, len(list))
	for i, x := range list {
		out[i] = map[string]any{"id": x.ID, "ip": x.IP, "user_agent": x.UserAgent, "created_at": x.CreatedAt,
			"last_seen_at": x.LastSeenAt, "expires_at": x.ExpiresAt, "current": cur != nil && cur.TokenHash == x.TokenHash}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	err := s.Store.DeleteSessionByID(r.Context(), userFrom(r.Context()).ID, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session introuvable")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Jetons API ---

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListAPITokens(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name          string   `json:"name"`
		TeamID        string   `json:"team_id"`
		Scopes        []string `json:"scopes"`
		ExpiresInDays int      `json:"expires_in_days"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "nom requis")
		return
	}
	if len(in.Scopes) == 0 {
		in.Scopes = []string{"read"}
	}
	for _, sc := range in.Scopes {
		if !slices.Contains(rbac.TokenScopes, sc) {
			writeError(w, http.StatusUnprocessableEntity, "scope inconnu : "+sc)
			return
		}
	}
	// Un jeton est rattaché à une équipe (l'équipe personnelle par défaut).
	ctx := r.Context()
	u := userFrom(ctx)
	if in.TeamID == "" {
		teams, err := s.Store.UserTeams(ctx, u.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		for _, t := range teams {
			if t.Personal {
				in.TeamID = t.ID
			}
		}
	}
	if _, err := s.enterTeam(r, in.TeamID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "équipe inconnue")
		return
	}
	t := &store.APIToken{UserID: u.ID, TeamID: &in.TeamID, Name: in.Name, Scopes: in.Scopes}
	if in.ExpiresInDays > 0 {
		exp := time.Now().AddDate(0, 0, in.ExpiresInDays)
		t.ExpiresAt = &exp
	}
	plain := auth.NewToken(auth.PrefixAPIToken)
	t.TokenHash = auth.HashToken(plain)
	created, err := s.Store.CreateAPIToken(r.Context(), t)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, created.TeamID, "auth.token.created", "api_token", created.ID, map[string]any{"name": created.Name, "scopes": created.Scopes})
	writeJSON(w, http.StatusCreated, map[string]any{"token": plain, "api_token": created})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "jeton introuvable")
		return
	}
	err := s.Store.DeleteAPIToken(r.Context(), userFrom(r.Context()).ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "jeton introuvable")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "auth.token.deleted", "api_token", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// --- utilitaires ---

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID string, mfaPending bool) error {
	tok := auth.NewToken(auth.PrefixSession)
	ttl := sessionTTL
	if mfaPending {
		ttl = mfaTTL
	}
	sess := &store.Session{TokenHash: auth.HashToken(tok), UserID: userID, MFAPending: mfaPending,
		IP: clientIP(r), UserAgent: truncate(r.UserAgent(), 255), ExpiresAt: time.Now().Add(ttl)}
	if err := s.Store.CreateSession(r.Context(), sess); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure: s.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: sess.ExpiresAt})
	return nil
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: s.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

// withUser place l'utilisateur dans le contexte (pour l'audit avant l'ouverture de session).
func withUser(r *http.Request, u *store.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxUser, u))
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide : "+err.Error())
		return false
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
