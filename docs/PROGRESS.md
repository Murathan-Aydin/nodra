# Suivi d'avancement Nodra

> **À lire en premier par toute nouvelle session Claude.** Mettre ce fichier à jour à CHAQUE modification (fait / reste à faire / décisions).
> Plan complet : [PLAN.md](PLAN.md). Contrat API : [API.md](API.md).

## Décisions
- Repartir de zéro (2026-10-30). Uniquement la version open source auto-hébergée de Coolify (`~/dev/coolify` sert de référence) : pas de Stripe, providers cloud, GitHub runners, DNS managé, Swarm.
- PostgreSQL + River (pas de Redis). Pas de Swagger : contrat dans `docs/API.md`.
- Requêtes SQL : pgx écrit à la main (sqlc non installé) ; migrations goose embarquées (`internal/store/migrations`, numérotées `000NN_nom.sql`).
- Équipes avancées : rôles personnalisés, permissions par ressource, sous-équipes/groupes, audit, approbations.
- README crédite Coolify comme inspiration.
- Module Go : `github.com/Murathan-Aydin/nodra`, dépôt `https://github.com/Murathan-Aydin/nodra` (push direct sur `main`).
- Auth : argon2id, sessions en base (seul le hash SHA-256 du jeton est stocké), cookie SameSite=Lax + garde CSRF (JSON obligatoire ou `X-Requested-With`), TOTP implémenté maison (RFC 6238, testé sur le vecteur officiel). `X-Forwarded-For` n'est pris en compte que si `NODRA_TRUST_PROXY=true`.

## Lancer / tester
```bash
docker compose -f docker-compose.dev.yml up -d
NODRA_ENV=development NODRA_DATABASE_URL='postgres://nodra:nodra@localhost:5433/nodra?sslmode=disable' \
NODRA_APP_KEY=$(openssl rand -base64 32) go run ./cmd/nodra serve
# tests d'intégration (créent puis suppriment une base temporaire) :
NODRA_TEST_DATABASE_URL='postgres://nodra:nodra@localhost:5433/nodra?sslmode=disable' go test -race ./...
```
La CI GitHub Actions (`.github/workflows/ci.yml`) lance gofmt, vet et les tests avec un Postgres.

## Fait
- **Étape 1 — Socle** (2026-10-30) : module Go `github.com/Murathan-Aydin/nodra`, config par env (`internal/config`), logs JSON `slog`, chiffrement AES-256-GCM (`internal/crypto`, testé), pool pgx + migrations goose embarquées (`internal/store`, table `instance_settings`), routeur chi avec `/api/v1/health` et service du front embarqué en SPA (`web/embed.go`, placeholder `web/dist/index.html`), arrêt gracieux, `docker-compose.dev.yml`, `.env.example`, README. Vérifié : health OK et fallback SPA en local.
- **Étape 2 — Auth & utilisateurs** (2026-10-30) : migration `00002_auth.sql` (users, sessions, api_tokens, password_resets) ; `internal/auth` (argon2id, jetons opaques préfixés `nds_`/`ndt_`/`ndr_`, TOTP + codes de secours) ; `internal/store` (users, sessions, tokens, instance) ; `internal/api` (middleware authenticate/requireUser/requireSession, garde CSRF, limite de débit, handlers). Fonctionnalités : inscription (1er user = admin d'instance, respect de `registration_enabled`), login, 2FA TOTP en deux temps (session `mfa_pending` de 10 min puis rotation), codes de secours à usage unique, logout, profil, changement de mot de passe (révoque les autres sessions), mot de passe oublié/réinitialisation (réponse constante, jeton 1 h), liste/révocation des sessions, jetons API avec scopes et expiration. Purge horaire des sessions expirées. Contrat dans `docs/API.md`. Tests : `internal/api/api_test.go` (parcours complet, CSRF, inscriptions fermées, limiteur). CI GitHub Actions ajoutée.

- **Étape 3 — Équipes avancées** (2026-10-30) : migration `00003_teams.sql` (teams avec `parent_id`, team_members, roles, groups, group_members, role_assignments, team_invitations, audit_events immuable par trigger, approval_rules/requests/votes, `api_tokens.team_id`). Package `internal/rbac` (catalogue de 42 permissions, rôles système owner/admin/developer/viewer synchronisés en base au démarrage, jokers, résolution `Decide` : owner > portée la plus précise > deny à portée égale, scopes de jetons). `internal/store/scopes.go` : `ScopeChain` extensible via `scopeParents` (chaque module y enregistre sa portée parente). API (`internal/api/authz.go`, `teams.go`, `access.go`, `audit.go`, `approvals.go`) : équipe personnelle à l'inscription, organisations + sous-équipes, membres (directs/hérités), invitations par lien liées à l'e-mail, rôles personnalisés, attributions user/groupe × portée × allow/deny, groupes, anti-élévation de privilèges, transfert de propriété, protection du dernier propriétaire, audit filtrable + export CSV (+ événements `auth.*`), règles et demandes d'approbation avec exécution automatique via `Server.Executors` et `s.gate(...)`. Tests : `internal/rbac/rbac_test.go`, `internal/api/teams_test.go` (`TestTeams`, `TestApprovals`).

- **Étape 4 — Clés SSH & serveurs** (2026-10-30) : migration `00004_servers.sql` (private_keys chiffrées, servers + settings JSON, destinations, server_tasks, server_metrics). `internal/ssh` : pool de connexions réutilisées, clé d'hôte enregistrée à la 1re connexion puis vérifiée strictement (Coolify ne vérifie pas), annulation par contexte, génération/import de clés, `Quote` ; `internal/ssh/sshtest` : serveur SSH en mémoire pour les tests. `internal/servers` : tâches validate, install_docker, docker_cleanup, packages_check/update, reboot, command, cloudflared_start/stop, network_create/delete, sortie écrite en base par lots ; métriques via SSH (/proc, df) sans agent. `internal/jobs` : **River** introduit (migrations River, file `servers`, jobs périodiques métriques 1 min / planificateur de nettoyage 5 min / maintenance 1 h, enfilement transactionnel tâche + job) ; remplace l'ancienne boucle de purge des sessions. API `keys.go`, `servers.go` ; portée `server` branchée dans `scopeParents` ; suppression de serveur soumise aux approbations (exécuteur `server.delete`). Tests : `internal/ssh`, `internal/servers`, `internal/jobs`, `internal/api/servers_test.go`. **Vérifié en vrai** : binaire lancé en local + conteneur Alpine avec sshd et la CLI Docker → validation, commande et métriques périodiques OK.

- **Étape 5 — Proxy** (2026-10-30) : migration `00005_proxy.sql` (colonnes proxy_* sur servers, proxy_configs, ssl_certificates). `internal/servers/proxy.go` : compose Traefik/Caddy générés (alignés sur Coolify), compose personnalisé validé (YAML + conteneur `nodra-proxy`), fichiers dynamiques, certificats écrits sur le serveur par un script qui supprime aussi les fichiers obsolètes (testé en l'exécutant réellement), tâches proxy_start/stop/restart/sync, vérification des ports 80/443, démarrage automatique après validation, statut du proxy relevé avec les métriques. API `proxy.go` : proxy, configs dynamiques, certificats (vérification clé/certificat/expiration), vérification DNS (résolveur injectable, jokers, Cloudflare Tunnel). Tests : `internal/servers/proxy_test.go`, `internal/api/proxy_test.go`. **Vérifié en vrai** : sur la machine de dev, les ports 80/443 sont pris par un `coolify-proxy` → Nodra le détecte, ne démarre pas son proxy et la validation reste réussie. Script e2e réutilisable : voir « Notes pour la reprise ».

- **Étape 6 — Projets & environnements** (2026-10-30) : migration `00006_projects.sql` (projects, environments, shared_variables chiffrées avec portée team/project/environment/server, tags). Portées `project` et `environment` branchées dans `scopeParents` → les droits par projet/environnement fonctionnent de bout en bout. API `projects.go` : projets (environnement production par défaut, liste filtrée par droits), environnements, clonage (avec variables), suppressions soumises aux approbations (exécuteurs `project.delete`, `environment.delete`) qui nettoient variables et attributions de rôle liées, variables partagées (secrets jamais réaffichés), tags. `store.SharedVariablesFor` prépare l'interpolation `{{team.X}}`… pour l'étape 8. Tests : `internal/api/projects_test.go`.

## Comment brancher un nouveau module sur le RBAC (à suivre aux étapes suivantes)
1. Routes sous `/teams/{team}/...` (middleware `withTeam` déjà en place).
2. Enregistrer la portée parente dans `scopeParents` (ex. `scopeParents["project"] = func(...) { return Scope{team, project.team_id} }`) pour que `ScopeChain` fonctionne.
3. Dans chaque handler : `s.authorize(w, r, "<permission>", rbac.Scope{Type, ID})`, et filtrer les requêtes SQL par `team_id` de l'équipe (ou sous-arbre).
4. Actions soumises à approbation (`rbac.GateableActions`) : `if req, err := s.gate(r, action, scope, résumé, payload); req != nil { respondPending(w, req); return }` puis enregistrer l'exécuteur correspondant dans `Server.Executors` (rejoue l'action depuis le payload).
5. `s.auditTeam(r, "<module>.<action>", "<type>", id, meta)` sur chaque mutation.

## Reste à faire (backend)
- [x] 2. Auth & utilisateurs — **reste** : vérification d'e-mail et envoi réel du mail de reset (attend l'étape 13, interface `api.Mailer` déjà prête), connexion OAuth (GitHub/GitLab/Google), rattachement des jetons API à une équipe (étape 3)
- [x] 3. Équipes avancées — **reste** : envoi réel des e-mails d'invitation (étape 13, `Mailer.TeamInvitation` prêt), notifications aux approbateurs (étape 13), nettoyage des ressources sur les serveurs à la suppression d'une équipe (étapes 8-10), consultation de l'audit `auth.*` par l'admin d'instance (étape 15)
- [x] 4. Clés SSH & serveurs — **reste** : terminal web interactif (WebSocket, étape 14), serveur « localhost » de l'instance (avec install.sh), alertes disque/injoignable (étape 13), annulation d'une tâche déjà en cours
- [x] 5. Proxy Traefik/Caddy — **reste** : labels de routage par ressource et redirections HTTP→HTTPS (étape 8), logs du proxy en direct (étape 14), certificats wildcard DNS-01 (OVH/Cloudflare, prévu en v1.1)
- [x] 6. Projets & environnements — **reste** : clonage des ressources avec l'environnement (étapes 8-10), étiquetage des ressources (étape 8)
- [ ] 7. Sources Git
- [ ] 8. Applications & déploiements
- [ ] 9. Bases de données
- [ ] 10. Services one-click
- [ ] 11. S3 & sauvegardes
- [ ] 12. Tâches planifiées
- [ ] 13. Notifications
- [ ] 14. Temps réel (WebSocket)
- [ ] 15. Réglages d'instance
- [ ] 16. API publique complète (+ MCP optionnel)
- [ ] Dockerfile + install.sh + CI GitHub Actions

## Reste à faire (front React, phase 2)
- [ ] Socle, dashboard, serveurs, projets/ressources, logs/terminal, catalogue services, équipes, paramètres

## Prochaine action
Étape 7 : sources Git. Prévu : table `git_sources` (team_id, type github|gitlab|gitea|bitbucket?, nom, URL API/HTML, App GitHub : app_id, client_id, client_secret chiffré, webhook_secret chiffré, private_key chiffrée, installation_id ; GitLab/Gitea : jeton d'accès chiffré, webhook_secret), création d'une GitHub App via le flux « manifest » (redirection + callback), liste des dépôts et branches via l'API, jetons d'installation GitHub (JWT RS256), deploy keys (réutiliser private_keys), webhooks entrants `/webhooks/github|gitlab|gitea` (vérification HMAC/jeton, push et pull requests → déclencheront les déploiements à l'étape 8). Référence Coolify : `app/Models/GithubApp.php`, `GitlabApp.php`, `GiteaApp.php`, `app/Http/Controllers/Webhook/*`, `routes/webhooks.php`.

## Notes pour la reprise
- Le serveur de test Oracle (84.235.231.3) fait tourner l'**ancienne** version de Nodra, qui suit le même dépôt GitHub : ne pas lancer `deploy/update.sh` dessus avant que la nouvelle version ait son install/update (étape « Dockerfile + install.sh »). La clé SSH `.dev/vm_ed25519` n'existe plus en local.
- Tests de bout en bout locaux : image `nodra-e2e-ssh` (Dockerfile Alpine `docker:27-cli` + `apk add --upgrade openssh-client-common openssh-client-default openssh-server`, `sed -i 's/^root:!/root:*/' /etc/shadow`, socket Docker monté, port 2299) pour simuler un serveur ; lancer Nodra sur :8098 avec une base `nodra_e2e`, attendre `/api/v1/health`, piloter l'API avec curl, puis supprimer le conteneur, le réseau Docker `nodra` et la base. Sur la machine de dev, les ports 80/443 sont occupés par le Coolify local de l'utilisateur : ne pas y toucher.
