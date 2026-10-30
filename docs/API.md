# API Nodra — v1

Base : `/api/v1`. Corps et réponses en JSON. Erreurs : `{"error": "message"}`.

## Authentification

Deux moyens :
- **Session** (navigateur) : cookie `nodra_session` (HttpOnly, SameSite=Lax, Secure hors développement), obtenu via `register`, `login` ou `login/2fa`. Durée 30 jours.
- **Jeton API** : en-tête `Authorization: Bearer ndt_…`. Les jetons ne peuvent pas gérer le compte (routes `/me/*` autres que `GET /me` → `403`).

Protection CSRF (requêtes par cookie) : toute requête modifiante doit avoir `Content-Type: application/json`, ou, sans corps, l'en-tête `X-Requested-With`.

Limite : 10 tentatives par minute et par IP sur les routes `/auth/*` (sauf `logout`) → `429`.
Derrière un reverse proxy, définir `NODRA_TRUST_PROXY=true` pour utiliser l'IP réelle du client.

## Santé

| Méthode | Route | Réponse |
|---|---|---|
| GET | `/health` | `200 {"status":"ok"}` ou `503 {"status":"degraded","database":"down"}` |

## Auth (`/auth`)

| Méthode | Route | Corps | Réponse |
|---|---|---|---|
| POST | `/auth/register` | `{email, password, name?}` | `201 User` + cookie. Le 1er utilisateur devient admin de l'instance. `403` si inscriptions fermées, `409` si e-mail pris, `422` si invalide (mot de passe ≥ 8 caractères). |
| POST | `/auth/login` | `{email, password}` | `200 {"mfa_required": false, "user": User}` + cookie, ou `{"mfa_required": true}` + cookie temporaire (10 min) si 2FA. `401` sinon. |
| POST | `/auth/login/2fa` | `{code}` ou `{recovery_code}` | `200 {"user": User}` + nouveau cookie. `401` si code invalide. Un code de secours n'est utilisable qu'une fois. |
| POST | `/auth/logout` | — | `204` |
| POST | `/auth/password/forgot` | `{email}` | `204` toujours (ne révèle pas l'existence du compte). Jeton valable 1 h, envoyé par e-mail (journalisé tant que l'e-mail n'est pas configuré). |
| POST | `/auth/password/reset` | `{token, password}` | `204`, révoque toutes les sessions. `400` si jeton invalide/expiré/déjà utilisé. |

## Compte (`/me`, authentifié)

| Méthode | Route | Corps | Réponse |
|---|---|---|---|
| GET | `/me` | — | `200 {"user": User, "two_factor_enabled": bool}` (session ou jeton) |
| PATCH | `/me` | `{name}` | `204` |
| POST | `/me/password` | `{current_password, new_password}` | `204`, révoque les autres sessions. `401` si mot de passe actuel faux. |
| POST | `/me/2fa/setup` | — | `200 {secret, otpauth_url}` (à afficher en QR code) |
| POST | `/me/2fa/enable` | `{code}` | `200 {"recovery_codes": [8 codes]}` (affichés une seule fois). `422` si code faux. |
| POST | `/me/2fa/disable` | `{password}` | `204` |
| GET | `/me/sessions` | — | `200 [{id, ip, user_agent, created_at, last_seen_at, expires_at, current}]` |
| DELETE | `/me/sessions/{id}` | — | `204` / `404` |
| GET | `/me/tokens` | — | `200 [APIToken]` |
| POST | `/me/tokens` | `{name, scopes?: ["read"|"write"|"deploy"|"root"], expires_in_days?}` | `201 {"token": "ndt_…", "api_token": APIToken}` — le jeton en clair n'est renvoyé qu'ici. |
| DELETE | `/me/tokens/{id}` | — | `204` / `404` |

## Objets

- **User** : `{id, email, name, is_instance_admin, email_verified_at, created_at}`
- **APIToken** : `{id, name, scopes, expires_at, last_used_at, created_at}`
