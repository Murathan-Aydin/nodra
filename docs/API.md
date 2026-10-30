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

## Sources Git (`/teams/{team}/sources`)

Types : **GitHub App** (github.com ou GitHub Enterprise Server), **GitLab** (gitlab.com ou auto-hébergé, jeton `read_api` + `read_repository`), **Gitea/Forgejo** (jeton). Les secrets (clé de l'App, secret du webhook, secret client, jeton) sont chiffrés et jamais renvoyés. Les dépôts publics et les deploy keys n'ont pas besoin de source (configurés sur l'application).

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/sources` | `source.read` | Liste (sans secrets) |
| POST | `/sources` | `source.manage` | GitHub : `{type: "github", name, app_id, private_key, webhook_secret, installation_id?, client_id?, client_secret?, html_url?, api_url?, organization?}` ; GitLab/Gitea : `{type, name, access_token, html_url?, api_url?}` (`api_url` déduite : `/api/v3`, `/api/v4`, `/api/v1`) |
| POST | `/sources/github/manifest` | `source.manage` | `{name, organization?, html_url?, is_public?}` → `201 {source, form_action, manifest}` : le front soumet un formulaire POST vers `form_action` avec le champ `manifest` |
| GET | `/sources/{id}` | `source.read` | |
| PATCH | `/sources/{id}` | `source.manage` | Champs partiels ; un secret vide (`""`) l'efface |
| DELETE | `/sources/{id}` | `source.manage` | `409` si des applications l'utilisent |
| GET | `/sources/{id}/repositories` | `source.read` | `[{full_name, name, private, default_branch, html_url}]` ; `502` si la forge refuse |
| GET | `/sources/{id}/branches?repo=owner/name` | `source.read` | `["main", …]` |

**Source** : `{id, team_id, type, name, html_url, api_url, organization, is_public, app_id, app_slug, client_id, installation_id, configured, installed, has_private_key, has_webhook_secret, has_access_token, created_at}`.

### Création automatique d'une GitHub App

1. `POST /sources/github/manifest` → le navigateur poste le manifeste sur GitHub (nom `nodra-<nom>-NNNN`, permissions contents/metadata/emails/administration en lecture, pull requests en écriture, événements push et pull_request, webhook `<instance>/webhooks/github`).
2. GitHub redirige vers `GET /api/v1/sources/github/callback?code=&state=` (session requise, `source.manage`) : Nodra récupère les identifiants de l'App et redirige vers la page d'installation.
3. Après installation, GitHub redirige vers `GET /api/v1/sources/github/install?installation_id=&state=` → redirection vers `/sources/{id}?installed=1`.

L'URL publique de l'instance vient du réglage d'instance (FQDN), sinon de la requête.

## Webhooks

| Méthode | Route | Authentification | Description |
|---|---|---|---|
| POST | `/webhooks/github` | `X-Hub-Signature-256` (secret de l'App, App identifiée par `X-GitHub-Hook-Installation-Target-ID`) | `push` et `pull_request` → applications liées à cette source ; `installation` met à jour l'installation ; `ping` → `pong` |
| POST | `/webhooks/{github,gitlab,gitea,bitbucket}/manual` | Secret de l'application : `X-Hub-Signature-256` (GitHub), `X-Gitlab-Token`, `X-Gitea-Signature`/`X-Forgejo-Signature`, `X-Hub-Signature` (Bitbucket) | Webhook configuré à la main sur la forge ; seules les applications du dépôt dont le secret valide la signature sont concernées (`401` sinon) |
| GET/POST | `/webhooks/deploy/{app}` | Secret de l'application en `X-Nodra-Secret`, `Authorization: Bearer` ou `?secret=` | Déploiement depuis une CI ; `?force=1` reconstruit, `?commit=<sha>` ; `202 {application_id, action, deployment_id \| approval_request_id}` |

Réponse des webhooks de forge : `[{application_id, action: deploy|preview_deploy|preview_removed|skipped|approval_required, deployment_id?, approval_request_id?, reason?}]`.

**Push** : déploie si la branche est celle suivie, `auto_deploy` actif et un fichier modifié correspond à `watch_paths` (un motif par ligne, `*`, `**`, `?`, `!` pour exclure ; sans liste de fichiers, tout push compte). Le commit poussé est déployé (`trigger: webhook`).
**Pull request** (branche cible = branche suivie, `preview_deployments` actif ; PR de forks seulement avec `public_pr_deployments`) : ouverture/mise à jour → preview créée (domaines dérivés de `preview_url_template`, `{{pr_id}}`, `{{domain}}`, `{{random}}`) puis déployée (`trigger: preview`, conteneur `<app>-pr-<n>`) ; fermeture → déploiements annulés, conteneurs arrêtés, preview supprimée. Pour une source GitHub App, un commentaire est publié sur la PR (puis mis à jour) avec l'URL ou l'échec.
Les déploiements déclenchés par webhook passent par les mêmes règles d'approbation `resource.deploy` (demande sans demandeur).

## Applications (`/teams/{team}/applications`)

Build packs : `nixpacks` (détection automatique), `dockerfile` (dans le dépôt ou en ligne via `dockerfile`), `dockercompose` (dans le dépôt ou brut via `docker_compose_raw`), `dockerimage` (image publique ou privée), `static` (site statique servi par `static_image`, `nginx:alpine` par défaut).

Source du code : dépôt public (`git_repository` = URL https), source Git de l'équipe (`source_id` + `git_repository` = `owner/nom`), ou deploy key (`private_key_id` + URL SSH `git@hôte:owner/nom.git`).

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/applications?environment_id=` | `resource.read` (par application) | |
| POST | `/applications` | `resource.create` (environnement) + `server.read` (serveur de la destination) | Voir champs ci-dessous. Sans `domains`, un domaine est généré : sous-domaine du `wildcard_domain` du serveur, sinon `http://<nom>-xxxxxx.<ip>.sslip.io`. `409` si un domaine est déjà utilisé (sauf `force_domain_override: true`). |
| GET | `/applications/{id}` | `resource.read` | `{application, tags, server_id, project_id}` |
| PATCH | `/applications/{id}` | `resource.update` | Champs partiels ; `health_check`, `limits`, `settings` sont fusionnés |
| DELETE | `/applications/{id}` | `resource.delete` | Soumis aux approbations ; arrête les conteneurs puis supprime variables, stockages, tags et droits liés |
| GET | `/applications/{id}/webhooks` | `resource.update` | `{secret, urls: {github, gitlab, gitea, bitbucket, deploy}}` (webhooks manuels) |
| POST | `/applications/{id}/webhooks/rotate` | `resource.update` | Nouveau secret |
| PUT | `/applications/{id}/tags` | `resource.update` | `{tag_ids}` → tags |

**Champs** : `environment_id`, `destination_id` (création uniquement), `name`, `description`, `git_repository`, `git_branch` (`main`), `git_commit_sha` (`HEAD` ou SHA), `source_id`, `private_key_id`, `build_pack`, `docker_image`, `docker_image_tag`, `static_image`, `base_directory` (`/`), `publish_directory`, `dockerfile`, `dockerfile_location` (`/Dockerfile`), `dockerfile_target`, `docker_compose_location` (`/docker-compose.yaml`), `docker_compose_raw`, `docker_compose_domains` (`{service: [domaines]}`), `install_command`, `build_command`, `start_command`, `watch_paths`, `ports_exposes` (`3000` nixpacks, `80` sinon), `ports_mappings` (`8080:80`), `domains` (URL ; `https://` ajouté si absent ; un chemin est possible), `redirect` (`both`/`www`/`non-www`), `preview_url_template` (`{{pr_id}}.{{domain}}`), `custom_labels`, `custom_docker_run_options`, `pre_deployment_command`/`_container`, `post_deployment_command`/`_container`, `health_check`, `limits`, `settings`, `basic_auth_password` (écriture seule, stocké en bcrypt ; la réponse indique `basic_auth_password_set`), `force_domain_override`.

**health_check** (défauts) : `enabled` false, `type` http|cmd, `path` `/`, `port` (premier port exposé), `host` localhost, `method` GET, `return_code` 200, `scheme` http, `response_text`, `interval` 5, `timeout` 5, `retries` 10, `start_period` 5, `command`.

**limits** : `memory`, `memory_swap`, `memory_reservation` (`"0"` = illimité, ex. `"512m"`), `memory_swappiness` 60, `cpus` (`"0"`, ex. `"1.5"`), `cpuset`, `cpu_shares` 1024.

**settings** (défauts) : `auto_deploy` true, `force_https` true, `preview_deployments` false, `public_pr_deployments` false, `git_submodules` true, `git_lfs` true, `git_shallow_clone` true, `is_spa` false, `gzip` true, `strip_prefix` true, `consistent_container_name` false, `connect_to_docker_network` false, `custom_internal_name`, `disable_build_cache` false, `include_source_commit_in_build` false, `inject_build_args` true, `use_build_secrets` false, `include_default_variables` true, `docker_images_to_keep` 2, `stop_grace_period` 30, `debug`, `gpu_enabled`/`gpu_driver`/`gpu_count`/`gpu_device_ids`, `basic_auth_enabled`/`basic_auth_username`, `certificate_resolver` letsencrypt, `raw_compose_deployment`, `preserve_repository`, `redact_env_values_in_logs`, `permanent_redirect`, `container_label_escape` true, `include_timestamps`, `build_server_enabled`, `env_sorting`, `container_label_readonly` true, `rolling_update_drain_seconds`.

### Variables d'environnement (`/applications/{id}/envs`)

Valeurs chiffrées ; elles peuvent référencer les variables partagées (`{{team.X}}`, `{{project.X}}`, `{{environment.X}}`, `{{server.X}}`). Une variable `is_preview` remplace la valeur normale dans les prévisualisations de PR. Toute écriture est soumise aux règles d'approbation `resource.env.write` (les valeurs restent chiffrées dans la demande).

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/envs` | `resource.env.read` | Liste ; valeur vide pour un secret |
| POST | `/envs` | `resource.env.write` | `{key, value, is_buildtime?: true, is_runtime?: true, is_preview?, is_literal?, is_multiline?, is_secret?, comment?}` : crée ou remplace (clé + `is_preview`) → liste |
| POST | `/envs/bulk` | `resource.env.write` | `{content: ".env", is_preview?, is_buildtime?}` → liste |
| PATCH / DELETE | `/envs/{env}` | `resource.env.write` | → liste |

### Stockages (`/applications/{id}/storages`)

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/storages` | `resource.read` | |
| POST | `/storages` | `resource.update` | `{kind: "volume", mount_path, name?}` (volume Docker nommé), `{kind: "bind", host_path, mount_path}`, `{kind: "file", mount_path, content}` (1 Mo max.) ; `409` si le point de montage existe |
| PATCH | `/storages/{id}` | `resource.update` | `{content}` (fichiers) |
| DELETE | `/storages/{id}` | `resource.update` | |

### Déploiements (`/applications/{id}/…`)

Un déploiement est asynchrone (`202 Deployment`) ; on suit son statut et ses logs par interrogation (WebSocket à l'étape 14). Un seul déploiement à la fois par application ; au plus `concurrent_builds` par serveur (les autres attendent) ; durée maximale `deployment_timeout` du serveur. Les commandes `deploy`, `restart`, `rollback` sont soumises aux règles d'approbation `resource.deploy`.

| Méthode | Route | Permission | Description |
|---|---|---|---|
| POST | `/deploy` | `resource.deploy` | `{force_rebuild?, commit_sha?}` → `202 Deployment` (ou demande d'approbation) |
| POST | `/restart` (alias `/start`) | `resource.deploy` | Redéploie la dernière image sans reconstruction (reconstruit si aucune) |
| POST | `/rollback` | `resource.deploy` | `{tag}` : redéploie une image précédente (`GET /images`) |
| POST | `/stop` | `resource.deploy` | Arrête et supprime les conteneurs (images et volumes conservés) |
| GET | `/images` | `resource.read` | Images disponibles pour un retour arrière `[{tag, image, created, size, current}]` |
| GET | `/logs?lines=100&pr=0` | `resource.logs.read` | Logs des conteneurs `{logs}` |
| GET | `/deployments` | `resource.read` | 50 derniers déploiements |
| GET | `/deployments/{dep}` | `resource.read` | |
| GET | `/deployments/{dep}/logs?after=&debug=1` | `resource.logs.read` | `{status, logs: [{id, created_at, kind, hidden, message}]}` ; `after` = dernier id reçu ; `debug=1` inclut les lignes masquées (compose généré…) |
| POST | `/deployments/{dep}/cancel` | `resource.deploy` | En file : annulé ; en cours : interrompu (commande distante tuée) |

`DELETE /applications/{id}?delete_volumes=true` supprime aussi les volumes Docker.

### Prévisualisations de PR (`/applications/{id}/previews`)

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/previews` | `resource.read` | `[Preview]` |
| POST | `/previews` | `resource.deploy` | `{pr_number, title?, pr_url?}` : crée la preview et la déploie → `202 Deployment` (ou approbation) |
| GET | `/previews/{pr}` | `resource.read` | |
| PATCH | `/previews/{pr}` | `resource.update` | `{domains}` (pris en compte au prochain déploiement) |
| POST | `/previews/{pr}/deploy` | `resource.deploy` | `{force_rebuild?}` |
| POST | `/previews/{pr}/stop` | `resource.deploy` | Arrête les conteneurs |
| DELETE | `/previews/{pr}` | `resource.deploy` | Annule les déploiements, arrête les conteneurs, supprime la preview |

**Preview** : `{id, application_id, pr_number, pr_url, title, domains, status: stopped|running, last_commit, created_at, updated_at}`. Logs des conteneurs : `GET /logs?pr=<n>`.

**Deployment** : `{id, team_id, application_id, server_id, status: queued|in_progress|finished|failed|cancelled, trigger: manual|api|webhook|rollback|restart|approval|preview, branch, commit_sha, commit_message, pr_number, force_rebuild, rollback_image, image, error, cancel_requested, created_by, created_at, started_at, finished_at}`.

**Déroulement** : résolution du commit (`git ls-remote`, sans cloner) → si l'image `nodra/<app>:<commit>-<empreinte de config>` existe déjà, la construction est sautée → clone (submodules, LFS, PR) → build (`nixpacks build --out` puis `docker build` ; `docker build` ; image statique nginx avec option SPA ; `docker pull` pour une image) avec labels `nodra.*` et `--build-arg` → commande de pré-déploiement dans le conteneur en cours → démarrage (`docker compose -p <déploiement>`) → attente de santé → suppression de l'ancien conteneur (bascule sans coupure ; arrêt préalable si ports publiés, nom fixe ou PR) → commande de post-déploiement → conservation des `docker_images_to_keep` dernières images. Les secrets (variables secrètes, jetons de clonage, deploy key) sont masqués dans les logs et les scripts passent par l'entrée standard SSH (absents de `ps` sur le serveur). Le statut des applications (`running`, `degraded`, `restarting`, `exited`, `stopped`) est relevé chaque minute.

## Bases de données (`/teams/{team}/databases`)

Types (image par défaut, port interne) : `postgresql` (`postgres:17-alpine`, 5432), `mysql` (`mysql:8`, 3306), `mariadb` (`mariadb:11`, 3306), `mongodb` (`mongo:7`, 27017), `redis` (`redis:7.2`, 6379), `keydb` (`eqalpha/keydb:latest`, 6379), `dragonfly` (`docker.dragonflydb.io/dragonflydb/dragonfly`, 6379), `clickhouse` (`clickhouse/clickhouse-server:25.8`, 9000).

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/databases?environment_id=` | `resource.read` (par base) | Sans identifiants |
| POST | `/databases` | `resource.create` (environnement) + `server.read` | `201 Database`. Mots de passe générés s'ils sont absents (32 caractères alphanumériques) ; volumes de données créés (`<type>-<data>-<id>`). `409` si un port public ou publié est déjà utilisé sur le serveur (bases publiques, `ports_mappings`, port SSH). |
| GET | `/databases/{id}` | `resource.read` | `{database, tags, server_id, project_id}` |
| PATCH | `/databases/{id}` | `resource.update` (+ `resource.env.write` pour changer un mot de passe) | Champs partiels ; `limits` et `settings` fusionnés. Pris en compte au prochain démarrage, sauf `is_public`/`public_port` appliqués aussitôt (tâche `database_proxy`) si la base tourne. |
| DELETE | `/databases/{id}?delete_volumes=true` | `resource.delete` | Soumis aux approbations ; supprime conteneurs, fichiers, (volumes), variables, stockages, tags et droits liés |
| POST | `/databases/{id}/start` · `/restart` | `resource.deploy` | Soumis aux règles d'approbation `resource.deploy` → `202 ServerTask` (ou demande d'approbation). `409` si une action est déjà en cours ou si le serveur n'est pas validé. |
| POST | `/databases/{id}/stop` | `resource.deploy` | `202 ServerTask` : arrête et supprime les conteneurs (volumes conservés) |
| GET | `/databases/{id}/tasks` | `resource.logs.read` | 50 dernières actions `[ServerTask]` (`kind` : `database_start`, `database_stop`, `database_restart`, `database_proxy` ; sortie dans `output`) |
| GET | `/databases/{id}/logs?lines=100` | `resource.logs.read` | Logs des conteneurs (base et proxy) `{logs}` |
| PUT | `/databases/{id}/tags` | `resource.update` | `{tag_ids}` → tags |
| … | `/databases/{id}/envs`, `/storages` | comme pour les applications | Variables ajoutées à l'environnement du conteneur (elles remplacent les variables générées de même nom ; pas de valeurs de preview) ; stockages volume/bind/fichier |

**Champs** : `environment_id`, `destination_id`, `type` (création uniquement), `name`, `description`, `image`, `username` (défaut `postgres`, `mysql`, `mariadb`, `root` pour MongoDB, `default` pour ClickHouse ; non configurable pour Redis/KeyDB/Dragonfly ; `root` interdit pour MySQL/MariaDB), `password` et `root_password` (MySQL/MariaDB) en écriture (8 à 256 caractères), `database_name` (défaut `postgres` ou `default`), `custom_config` (postgresql.conf, my.cnf, mongod.conf, redis.conf, keydb.conf ; 64 Ko), `init_scripts` (`[{filename, content}]`, PostgreSQL/MySQL/MariaDB/MongoDB, 1 Mo au total ; exécutés par l'image au premier démarrage uniquement), `is_public` + `public_port` (hors 80/443), `ports_mappings` (`5433:5432`), `custom_docker_run_options`, `limits` (comme les applications), `settings` : `health_check_enabled` (true), `stop_grace_period` (30), `postgres_initdb_args`, `postgres_host_auth_method`.

**Database** : les champs ci-dessus (sans les mots de passe) + `id`, `team_id`, `status` (`stopped`, `running`, `degraded`, `restarting`, `exited`, relevé chaque minute), `started_at`, `created_at`, `updated_at`, `internal_host` (= `id`, joignable par les ressources du même réseau Docker) et `internal_port`. Avec `resource.env.read` : `password`, `root_password`, `internal_url` (ex. `postgres://user:pass@<id>:5432/db`, `redis://default:pass@<id>:6379/0`) et `public_url` (IP du serveur et port public) si la base est publique.

**Démarrage** (tâche serveur) : fichiers écrits dans `/data/nodra/databases/<id>` (compose, configuration, scripts d'initialisation, fichiers), `docker compose pull` puis `up -d` (conteneur `<id>`, labels `nodra.*`, réseau de la destination, vérification de santé propre au type lisant les identifiants dans l'environnement du conteneur), attente de santé (3 min max.), puis proxy public : conteneur `nginx:stable-alpine` `<id>-proxy` (module stream, résolution par le DNS de Docker) publiant `public_port`. Les mots de passe ne sont appliqués par la plupart des images qu'à l'initialisation des données ; Redis, KeyDB et Dragonfly les prennent au redémarrage.

## Services one-click (`/teams/{team}/services`)

Catalogue de 347 modèles docker-compose repris de Coolify (`internal/services/templates`, Apache-2.0). Variables magiques reconnues : `SERVICE_URL_<ID>[_<PORT>]` (URL publique) et `SERVICE_FQDN_<ID>[_<PORT>]` (hôte) — déclarées sans valeur dans un service, elles l'exposent sur le port indiqué et un domaine est généré (sous-domaine du `wildcard_domain` du serveur, sinon `sslip.io`) ; `SERVICE_USER_*`, `SERVICE_PASSWORD[_64]_*`, `SERVICE_PASSWORDWITHSYMBOLS[_64]_*`, `SERVICE_BASE64[_32|_64|_128]_*`, `SERVICE_REALBASE64[_32|_64|_128]_*`, `SERVICE_HEX_32|64|128_*`, `SERVICE_LOWERCASEUSER_*`, `SERVICE_SUPABASEANON_*`/`SUPABASESERVICE_*` (JWT signés avec `SERVICE_PASSWORD_JWT`) sont générées une fois à la création et stockées comme variables du service (modifiables). Les autres `${VAR:-défaut}` deviennent des variables configurables. Extensions Coolify prises en charge : volumes `content` / `isDirectory` (fichiers créés sous le dossier du service, sources relatives uniquement), `exclude_from_hc`.

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/service-templates?q=&category=` (racine) | connecté | `{templates: [{slug, name, slogan, documentation, category, tags, logo, port}], categories}` |
| GET | `/service-templates/{slug}` (racine) | connecté | Modèle avec `compose` |
| GET | `/services?environment_id=` | `resource.read` | |
| POST | `/services` | `resource.create` (environnement) + `server.read` | `{environment_id, destination_id, template? \| docker_compose_raw, name?, description?, instant_deploy?}` → `201 {service, apps, tags, task?, approval_request?}` |
| GET | `/services/{id}` | `resource.read` | `{service, apps, tags}` |
| PATCH | `/services/{id}` | `resource.update` | `{name?, description?, docker_compose_raw?}` ; un nouveau compose est ré-analysé (nouveaux secrets générés, valeurs et domaines existants conservés) |
| DELETE | `/services/{id}?delete_volumes=true` | `resource.delete` | Soumis aux approbations |
| PATCH | `/services/{id}/apps/{nom}` | `resource.update` | `{domains?, port?, exclude_from_status?, force_domain_override?}` (port du conteneur dans le domaine possible) |
| POST | `/services/{id}/start` \| `/restart` | `resource.deploy` | Tâche serveur ; soumis aux approbations |
| POST | `/services/{id}/stop` | `resource.deploy` | |
| POST | `/services/{id}/apps/{nom}/restart` | `resource.deploy` | Redémarre un conteneur |
| GET | `/services/{id}/tasks` | `resource.logs.read` | Tâches serveur du service |
| GET | `/services/{id}/logs?app=&lines=` | `resource.logs.read` | |
| PUT | `/services/{id}/tags` | `resource.update` | |
| … | `/services/{id}/envs…` | comme les applications | Variables (générées et configurables) |

Au démarrage, Nodra écrit dans `/data/nodra/services/<id>/` le compose transformé (`KEY` sans valeur → `KEY: ${KEY}`, conteneurs `<service>-<id>`, labels `nodra.*` et de routage, réseau de la destination sauf `network_mode`, volumes nommés déclarés, montages relatifs vers `data/`), le `.env` (variables, partagées interpolées, `SERVICE_URL_*`/`SERVICE_FQDN_*` recalculées depuis les domaines) et les fichiers générés, puis `docker compose pull` + `up -d` et attend les conteneurs (hors exclus).

## Stockages S3 (`/teams/{team}/s3-storages`)

Tout stockage compatible S3 (AWS, Cloudflare R2, Garage, SeaweedFS, MinIO…). L'accès au bucket est vérifié à la création et à la modification (`is_usable`, `last_error`). Clés chiffrées, jamais renvoyées.

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/s3-storages` | `storage.read` | |
| POST | `/s3-storages` | `storage.manage` | `{name, endpoint, bucket, access_key, secret_key, region?: "us-east-1", path_prefix?, description?}` |
| GET / PATCH / DELETE | `/s3-storages/{id}` | `storage.read` / `storage.manage` | Les clés vides ne sont pas modifiées |
| POST | `/s3-storages/{id}/check` | `storage.manage` | Revérifie l'accès |

## Sauvegardes (`/teams/{team}/backups`, `/backup-executions`)

Dumps de PostgreSQL (`pg_dump` format custom, ou `pg_dumpall` compressé), MySQL / MariaDB (`mysqldump` / `mariadb-dump` compressé), MongoDB (`mongodump --archive --gzip`), Redis / KeyDB (fichier RDB, restauration manuelle), et archives de volumes Docker (`tar.gz`). Les commandes s'exécutent dans le conteneur avec ses propres identifiants (aucun mot de passe dans les arguments visibles sur l'hôte). L'envoi vers S3 se fait depuis le serveur avec `rclone/rclone:1`, configuré par variables d'environnement. Fichiers locaux : `<NODRA_SERVER_DATA_DIR>/backups/<ressource>/`.

| Méthode | Route | Permission | Description |
|---|---|---|---|
| GET | `/backups?resource_id=` | `resource.read` | Planifications d'une ressource |
| POST | `/backups` | `resource.backup.manage` | `{kind: "database"\|"volume", resource_id, volume_name?, databases? ("a,b"), dump_all?, frequency?: "0 0 * * *", enabled?, keep_local?: 7, keep_local_days?, local_backup?: true, s3_storage_id?, keep_s3?: 30, keep_s3_days?}` ; un volume doit appartenir à la ressource |
| PATCH / DELETE | `/backups/{id}` | `resource.backup.manage` | |
| POST | `/backups/{id}/run` | `resource.backup.manage` | Sauvegarde immédiate (tâche serveur) |
| GET | `/backup-executions?resource_id=` | `resource.read` | `[{id, schedule_id, database_name, status, filename, size, s3_key, local_deleted, s3_deleted, message, created_at, finished_at}]` |
| POST | `/backup-executions/{id}/restore` | `resource.backup.restore` | Soumis aux approbations ; récupère le fichier depuis S3 si la copie locale n'existe plus |
| GET | `/backup-executions/{id}/download` | `resource.backup.manage` | `{url}` présignée (15 min) si sur S3, sinon le fichier lu sur le serveur |
| DELETE | `/backup-executions/{id}` | `resource.backup.manage` | Supprime fichier local et objet S3 |
| POST | `/databases/{id}/import?filename=&database=` | `resource.backup.restore` | Corps brut (`.dump`, `.sql`, `.sql.gz`, `.archive[.gz]`, 20 Go max., en-tête `X-Requested-With` requis par cookie) envoyé en flux vers le serveur puis restauré ; soumis aux approbations |

Planificateur : chaque minute, les planifications arrivées à échéance (fuseau horaire du serveur) sont lancées ; rétention locale et S3 appliquée après chaque exécution (nombre et/ou âge).
