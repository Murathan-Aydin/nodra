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
| POST | `/me/tokens` | `{name, team_id?, scopes?: ["read"|"write"|"deploy"|"root"], expires_in_days?}` (équipe personnelle par défaut) | `201 {"token": "ndt_…", "api_token": APIToken}` — le jeton en clair n'est renvoyé qu'ici. |
| DELETE | `/me/tokens/{id}` | — | `204` / `404` |

## Objets

- **User** : `{id, email, name, is_instance_admin, email_verified_at, created_at}`
- **APIToken** : `{id, team_id, name, scopes, expires_at, last_used_at, created_at}`

## Équipes, rôles et permissions

### Modèle

- **Organisation** = équipe racine ; **sous-équipe** = équipe avec `parent_id` (5 niveaux max.). Être membre d'une équipe donne accès à toutes ses sous-équipes ; un membre direct d'une sous-équipe ne voit pas l'organisation parente.
- Chaque utilisateur reçoit une **équipe personnelle** (non supprimable, impossible à quitter).
- **Permissions** atomiques (`GET /permissions` pour le catalogue), regroupées dans des **rôles** : système (`owner`, `admin`, `developer`, `viewer`, non modifiables) ou personnalisés (jokers `*` et `préfixe.*` acceptés).
- **Attributions** : un rôle donné à un utilisateur **ou à un groupe**, sur une **portée** (`team`, `project`, `environment`, `server`, `resource`), en `allow` ou `deny`.
- **Résolution** : un propriétaire a tous les droits ; sinon la portée la plus précise ayant une décision l'emporte (ressource > environnement > projet > équipe > équipe parente), et à portée égale `deny` l'emporte. Sans attribution : refus.
- **Anti-élévation** : on ne peut créer/modifier un rôle, attribuer un rôle, inviter avec un rôle ou ajouter quelqu'un à un groupe que si l'on détient soi-même toutes les permissions concernées. Le rôle `owner` ne s'attribue qu'à un utilisateur, sur l'équipe, par un propriétaire. Le dernier propriétaire ne peut être ni retiré ni rétrogradé.
- **Jetons API** : rattachés à une équipe (et ses sous-équipes) ; scopes `read` (lecture hors secrets), `write` (tout sauf déploiement, administration d'équipe et approbations), `deploy` (`resource.deploy`), `root` (tout). Les droits effectifs sont l'intersection du scope et du RBAC du propriétaire du jeton.

Une équipe inaccessible renvoie `404` (on ne révèle pas son existence) ; une permission manquante renvoie `403 {"error": "permission requise : <perm>"}`.

### Routes (toutes sous `/teams/{team}` sauf mention)

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/permissions` (racine) | — | Catalogue : `permissions`, `scope_types`, `token_scopes`, `gateable_actions` |
| GET | `/teams` (racine) | — | Équipes accessibles (directes + sous-équipes) |
| POST | `/teams` (racine) | `team.subteams.manage` sur le parent si `parent_id` | `{name, description?, parent_id?}` → `201 Team` ; le créateur devient propriétaire |
| GET | `/` | `team.read` | `{team, ancestors, permissions, is_owner}` |
| PATCH | `/` | `team.update` | `{name, description}` |
| DELETE | `/` | `team.delete` | `409` pour une équipe personnelle |
| GET | `/permissions?scope_type=&scope_id=` | membre | Permissions effectives de l'appelant sur une portée |
| POST | `/leave` | membre direct | `409` si dernier propriétaire ou équipe personnelle |
| POST | `/transfer-ownership` | propriétaire direct | `{user_id}` ; l'ancien propriétaire devient admin |
| GET | `/members` | `team.read` | `[{user_id, email, name, team_id, inherited, joined_at, roles}]` |
| POST | `/members` | `team.members.manage` | Sous-équipe uniquement : `{user_id, role_id}` (membre de l'équipe parente) |
| DELETE | `/members/{user}` | `team.members.manage` | Retire de l'équipe et des sous-équipes ; `409` si membre hérité |
| GET / POST | `/invitations` | `team.members.manage` | POST `{email, role_id, group_ids?}` → `201 {invitation, token}` (valable 7 jours, liée à l'e-mail) |
| DELETE | `/invitations/{id}` | `team.members.manage` | |
| POST | `/invitations/preview` (racine) | session | `{token}` → `{team_name, role_name, email, expires_at}` |
| POST | `/invitations/accept` (racine) | session | `{token}` → `200 Team` ; `403` si l'e-mail ne correspond pas |
| GET | `/roles` | `team.read` | Rôles système + rôles de l'équipe et de ses ancêtres |
| POST / PATCH / DELETE | `/roles`, `/roles/{id}` | `team.roles.manage` | `{name, description?, permissions}` ; rôles de l'équipe uniquement |
| GET | `/role-assignments` | `team.read` | Attributions définies par l'équipe |
| POST | `/role-assignments` | `team.roles.manage` **sur la portée visée** | `{user_id \| group_id, role_id, scope?: {type, id}, effect?: "allow"\|"deny"}` |
| DELETE | `/role-assignments/{id}` | `team.roles.manage` sur la portée | |
| GET / POST | `/groups` | `team.read` / `team.members.manage` | `{name, description?}` ; réponse avec `members` |
| PATCH / DELETE | `/groups/{id}` | `team.members.manage` | |
| PUT / DELETE | `/groups/{id}/members/{user}` | `team.members.manage` | |

### Journal d'audit

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/audit` | `team.audit.read` | Filtres : `actor` (user id), `action` (préfixe), `target_type`, `target_id`, `since`, `until` (RFC 3339), `before` (curseur), `limit` (≤ 200). → `{events, next_before}` |
| GET | `/audit/export` | `team.audit.read` | Mêmes filtres, CSV (10 000 lignes max.) |

Inclut les événements de l'équipe et de ses sous-équipes. Le journal est en ajout seul (trigger PostgreSQL) et survit à la suppression des équipes. Les événements d'authentification (`auth.*`) sont enregistrés sans équipe (consultables par l'admin d'instance à l'étape 15).

**AuditEvent** : `{id, team_id, actor_user_id, actor_email, actor_token_id, action, target_type, target_id, ip, user_agent, metadata, created_at}` (`metadata.before` / `metadata.after` pour les modifications).

### Approbations

Une **règle** soumet des actions (`gateable_actions` : `resource.deploy`, `resource.delete`, `resource.env.write`, `resource.backup.restore`, `environment.delete`, `project.delete`, `server.delete`) sur une portée à N approbations. Quand une action couverte est demandée, l'API répond `202 {"approval_required": true, "approval_request": ApprovalRequest}` au lieu de l'exécuter. Elle est exécutée automatiquement à la N-ième approbation ; un seul rejet suffit à la refuser. Le demandeur ne peut pas approuver sa propre demande. Les demandes expirent après 7 jours.

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/approval-rules` | `team.read` | |
| POST | `/approval-rules` | `approvals.manage` sur la portée | `{name, scope?, actions, required_approvals?: 1-10, enabled?}` |
| PATCH / DELETE | `/approval-rules/{id}` | `approvals.manage` | |
| GET | `/approvals?status=` | `team.read` | `pending`, `approved`, `rejected`, `cancelled`, `executed`, `failed`, `expired` |
| GET | `/approvals/{id}` | `team.read` | Avec `votes` |
| POST | `/approvals/{id}/approve` | `approvals.approve` sur la portée | `{comment?}` → demande mise à jour |
| POST | `/approvals/{id}/reject` | `approvals.approve` sur la portée | `{comment?}` |
| POST | `/approvals/{id}/cancel` | demandeur ou `approvals.manage` | |
