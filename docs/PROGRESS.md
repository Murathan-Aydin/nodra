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

## Reste à faire (backend)
- [x] 2. Auth & utilisateurs — **reste** : vérification d'e-mail et envoi réel du mail de reset (attend l'étape 13, interface `api.Mailer` déjà prête), connexion OAuth (GitHub/GitLab/Google), rattachement des jetons API à une équipe (étape 3)
- [ ] 3. Équipes avancées (orga/équipes/groupes, rôles perso, permissions par ressource, audit, approbations, invitations)
- [ ] 4. Clés SSH & serveurs
- [ ] 5. Proxy Traefik/Caddy
- [ ] 6. Projets & environnements
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
Étape 3 : équipes avancées. Prévu : tables `teams` (avec `parent_id` pour les sous-équipes), `team_members`, `groups` + `group_members`, `roles` (système + personnalisés, `permissions text[]`), `role_assignments` (sujet user|groupe, portée team|project|environment|server|resource, allow/deny), `invitations`, `audit_events`, `approval_rules` + `approval_requests`. Package `internal/rbac` avec le catalogue des permissions et la résolution « le plus spécifique gagne, deny prioritaire ». Créer une équipe personnelle à l'inscription. Ajouter `team_id` aux jetons API.
