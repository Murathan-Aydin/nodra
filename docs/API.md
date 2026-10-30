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

## Clés SSH (`/teams/{team}/private-keys`)

La partie privée est chiffrée en base et n'est jamais renvoyée.

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/private-keys` | `key.read` | `[PrivateKey]` |
| POST | `/private-keys` | `key.manage` | `{name, description?, private_key?}` : importe une clé PEM/OpenSSH (sans phrase de passe) ou, si `private_key` est vide, génère une paire ed25519. → `201 PrivateKey` |
| GET / PATCH / DELETE | `/private-keys/{id}` | `key.read` / `key.manage` | PATCH `{name, description}` ; DELETE → `409` si un serveur l'utilise |

**PrivateKey** : `{id, team_id, name, description, public_key, fingerprint, created_at}` — ajouter `public_key` au fichier `~/.ssh/authorized_keys` du serveur.

## Serveurs (`/teams/{team}/servers`)

Les opérations distantes sont des **tâches** asynchrones (`202 ServerTask`) dont on suit le statut et la sortie via `GET /servers/{id}/tasks/{task}`.

| Méthode | Route | Permission (portée serveur) | Description |
|---|---|---|---|
| GET | `/servers` | `server.read` (équipe) | |
| POST | `/servers` | `server.create` (équipe) | `{name, ip, port?: 22, user?: "root", private_key_id, description?, settings?}` → `201 {server, task}` (validation lancée automatiquement) |
| GET | `/servers/{id}` | `server.read` | `{server, destinations}` |
| PATCH | `/servers/{id}` | `server.update` | Champs partiels ; `settings` est fusionné. Changer IP/port/clé efface la clé d'hôte ; changer IP/port/utilisateur/clé repasse le serveur en non validé. |
| DELETE | `/servers/{id}` | `server.delete` | Soumis aux règles d'approbation (`server.delete`) ; `409` si des ressources y sont déployées |
| POST | `/servers/{id}/validate` | `server.update` | Connexion (enregistre la clé d'hôte à la 1re connexion), OS, Docker ≥ 24, Docker Compose, réseau `nodra` |
| POST | `/servers/{id}/install-docker` | `server.update` | Prérequis + Docker (get.docker.com, apk, pacman), rotation des logs, puis validation |
| POST | `/servers/{id}/docker-cleanup` | `server.update` | Conteneurs arrêtés, images inutilisées, cache de build (+ volumes/réseaux si activé) ; épargne tout ce qui porte `nodra.managed=true` |
| POST | `/servers/{id}/packages/check` | `server.update` | Liste des mises à jour système disponibles |
| POST | `/servers/{id}/packages/update` | `server.update` | Mise à jour des paquets |
| POST | `/servers/{id}/reboot` | `server.update` | |
| POST | `/servers/{id}/commands` | `server.terminal` | `{command}` : exécute une commande (enregistrée dans l'audit) |
| PUT / DELETE | `/servers/{id}/cloudflared` | `server.update` | PUT `{token}` démarre Cloudflare Tunnel (jeton chiffré, masqué dans la sortie) ; DELETE l'arrête |
| POST | `/servers/{id}/host-key/reset` | `server.update` | Oublie la clé d'hôte (serveur réinstallé) ; revalider ensuite |
| GET | `/servers/{id}/tasks` | `server.read` | 50 dernières tâches (sans `output`) |
| GET | `/servers/{id}/tasks/{task}` | `server.read` | Tâche avec sa sortie |
| POST | `/servers/{id}/tasks/{task}/cancel` | `server.update` | Uniquement si encore en file |
| GET | `/servers/{id}/metrics?since=` | `server.read` | Points `{ts, cpu_percent, mem_used, mem_total, disk_used, disk_total, load1}` (dernière heure par défaut) |
| GET / POST | `/servers/{id}/destinations` | `server.read` / `server.update` | POST `{name?, network}` crée un réseau Docker (tâche) |
| DELETE | `/servers/{id}/destinations/{dest}` | `server.update` | `409` pour la destination par défaut |

**Server** : `{id, team_id, name, description, ip, port, user, private_key_id, host_key, is_reachable, is_usable, os_id, os_version, docker_version, compose_version, validation_error, validated_at, last_seen_at, last_cleanup_at, settings, created_at}`

**settings** (valeurs par défaut) : `concurrent_builds` 2, `deployment_timeout` 3600 s, `is_build_server` false, `docker_cleanup_frequency` `"0 0 * * *"` (cron ou `@daily`…), `docker_cleanup_threshold` 80 (% disque déclenchant un nettoyage), `delete_unused_volumes` false, `delete_unused_networks` false, `metrics_enabled` true, `metrics_retention_days` 7, `timezone` `"UTC"`, `wildcard_domain` `""`, `connection_timeout` 10 s, `cloudflare_tunnel` (lecture seule).

**ServerTask** : `{id, team_id, server_id, kind, params, status: queued|running|success|failed|cancelled, output, error, created_by, created_at, started_at, finished_at}` ; `kind` ∈ `validate`, `install_docker`, `docker_cleanup`, `packages_check`, `packages_update`, `reboot`, `command`, `cloudflared_start`, `cloudflared_stop`, `network_create`, `network_delete`.

Tâches de fond automatiques : métriques et joignabilité chaque minute, nettoyage Docker selon `docker_cleanup_frequency` ou au-delà de `docker_cleanup_threshold` (vérifié toutes les 5 min), maintenance horaire (sessions expirées, rétention des métriques, demandes d'approbation expirées, tâches bloquées).

## Proxy (`/teams/{team}/servers/{id}/proxy`)

Chaque serveur a un proxy inverse : **Traefik** (par défaut, `traefik:v3.7`) ou **Caddy** (`lucaslorentz/caddy-docker-proxy:2.13-alpine`), ou aucun. Il tourne dans le conteneur `nodra-proxy` sur le réseau `nodra`, ports 80/443 (+ 443/udp pour HTTP/3), certificats Let's Encrypt (HTTP-01). Les fichiers sont dans `/data/nodra/proxy` sur le serveur. Le proxy est démarré automatiquement après une validation réussie (sans faire échouer la validation si les ports 80/443 sont pris par un autre conteneur ou service).

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/proxy` | `server.read` | `{type, status, acme_email, compose, is_custom, default_compose}` |
| PATCH | `/proxy` | `server.proxy.manage` | `{type?: traefik\|caddy\|none, acme_email?, compose?}` ; `compose: ""` revient à la configuration générée ; changer de type exige un proxy arrêté (`409`) |
| POST | `/proxy/start` \| `/proxy/stop` \| `/proxy/restart` | `server.proxy.manage` | Tâches ; le démarrage vérifie que les ports 80/443 sont libres |
| GET | `/proxy/configs` | `server.read` | Fichiers de configuration dynamique |
| PUT | `/proxy/configs/{name}` | `server.proxy.manage` | `{content}` ; `.yml`/`.yaml` pour Traefik (provider fichier), `.caddy` pour Caddy (importés) ; préfixe `nodra-` réservé. Synchronisé sur le serveur sans redémarrage. |
| DELETE | `/proxy/configs/{name}` | `server.proxy.manage` | |
| GET | `/proxy/certificates` | `server.read` | Certificats personnalisés `{id, name, domains, certificate, expires_at}` |
| POST | `/proxy/certificates` | `server.proxy.manage` | `{name, certificate (PEM, chaîne possible), private_key}` : vérifie la correspondance clé/certificat et l'expiration ; Traefik les charge automatiquement (`dynamic/nodra-certs.yml`), Caddy les reçoit dans `/certs` |
| DELETE | `/proxy/certificates/{cert}` | `server.proxy.manage` | |

`status` : `stopped`, `running`, `exited`… (mis à jour au démarrage/arrêt et chaque minute avec les métriques). À la création d'un serveur, `proxy_type` peut être fourni (`traefik` par défaut).

## Vérification DNS

| Méthode | Route | Permission | Description |
|---|---|---|---|
| POST | `/teams/{team}/dns-check` | `server.read` sur le serveur | `{domain, server_id}` → `{domain, resolved, expected, ok, message}`. Accepte une URL complète ; `*.exemple.com` teste un sous-domaine aléatoire. Avec Cloudflare Tunnel actif, le domaine est considéré correct. |

## Projets et environnements (`/teams/{team}/projects`)

Un projet est créé avec un environnement `production`. Les permissions peuvent être attribuées sur un projet ou un environnement (voir « Équipes, rôles et permissions »).

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/projects` | `project.read` (équipe ou projet) | Projets visibles par l'appelant |
| POST | `/projects` | `project.create` (équipe) | `{name, description?}` → `201 Project` |
| GET | `/projects/{p}` | `project.read` (projet ou un de ses environnements) | `{project, environments}` (environnements visibles) |
| PATCH | `/projects/{p}` | `project.update` | `{name, description?}` |
| DELETE | `/projects/{p}` | `project.delete` | Soumis aux approbations ; supprime environnements, variables et attributions liées ; `409` s'il reste des ressources |
| POST | `/projects/{p}/environments` | `environment.create` (projet) | `{name, description?}` |
| GET | `/projects/{p}/environments/{e}` | `project.read` (environnement) | `{project, environment}` |
| PATCH | `/projects/{p}/environments/{e}` | `environment.update` | |
| DELETE | `/projects/{p}/environments/{e}` | `environment.delete` | Soumis aux approbations ; `409` s'il reste des ressources |
| POST | `/projects/{p}/environments/{e}/clone` | `project.read` (source) + `environment.create` (projet cible) | `{name, project_id?}` : copie l'environnement et ses variables (les ressources seront copiées par leurs modules) |

## Variables partagées (`/teams/{team}/variables`)

Variables réutilisables dans les ressources via `{{team.CLÉ}}`, `{{project.CLÉ}}`, `{{environment.CLÉ}}`, `{{server.CLÉ}}`. Valeurs chiffrées en base.

| Méthode | Route | Permission (sur la portée) | Description |
|---|---|---|---|
| GET | `/variables?scope_type=&scope_id=` | `variable.read` | Équipe par défaut ; la valeur d'un secret est toujours vide |
| POST | `/variables` | `variable.manage` | `{scope?: {type: team\|project\|environment\|server, id}, key, value, comment?, is_secret?, is_multiline?, is_literal?}` ; `409` si la clé existe sur la portée |
| PATCH | `/variables/{id}` | `variable.manage` | Champs partiels ; un secret ne redevient visible qu'en redéfinissant sa valeur |
| DELETE | `/variables/{id}` | `variable.manage` | |

**SharedVariable** : `{id, team_id, scope, key, value, comment, is_secret, is_multiline, is_literal, updated_at}`

## Tags (`/teams/{team}/tags`)

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/tags` | `team.read` | |
| POST / PATCH / DELETE | `/tags`, `/tags/{id}` | `resource.update` | `{name, color?: "#rrggbb"}` (l'étiquetage des ressources arrive avec elles) |
