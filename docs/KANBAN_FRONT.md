# Kanban — front React de Nodra (phase 2)

Le backend (phase 1) est terminé : toutes les routes sont décrites dans [API.md](API.md). Ce kanban découpe le front en cartes réalisables une par une.

**Utilisation** : déplacer l'identifiant d'une carte de « À faire » vers « En cours » puis « Fait » (le détail des cartes reste en bas, par lot). Taille indicative : `S` ≤ ½ jour, `M` ≈ 1 jour, `L` 2-3 jours. Chaque carte liste les routes API utilisées (préfixe `/api/v1`, `/teams/{team}` sous-entendu pour les routes d'équipe) et le critère « fini quand ».

---

> **2026-10-31 — front remis à zéro.** Le front v1 (lots 0 à 10, codé par Claude) a été supprimé à la demande de l'utilisateur, qui a refait le visuel. Seul le système visuel est conservé dans `web/src` (`index.css`, `wallpapers.css`, `lib/theme.tsx` et l'Apparence, composants `components/ui/`). L'ancien front et la refonte sont archivés dans la branche locale `archive/front-v1`. **L'ordre de reconstruction est fixé par l'utilisateur** ; les cartes ci-dessous restent la référence des routes API à utiliser.

## 🟦 À faire

Toutes les cartes (lots 0 à 13), dans l'ordre que donnera l'utilisateur.

## 🟨 En cours

_(rien)_

## 🟩 Fait

**Premier démarrage** (demandé par l'utilisateur) : création du compte administrateur (`/setup`), configuration guidée (`/onboarding` : serveur « Cette machine » ou SSH, validation suivie, premier projet, récapitulatif), connexion avec double authentification (`/login`). Cadre de l'application (barre du haut, barre latérale, recherche ⌘K, menu utilisateur) et tableau de bord façon Coolify.

---

## Conventions communes (à lire avant de commencer)

- **Requêtes** : `credentials: "include"` (cookie de session `HttpOnly`), en-tête `X-Requested-With: nodra` sur toute requête qui modifie (garde CSRF), corps JSON. Erreurs : `{"error": "message en français"}` (à afficher tel quel).
- **Codes à traiter partout** : `401` → page de connexion (avec `redirect`) ; `403` → « droits insuffisants » ; `409` → conflit métier (message) ; `422` → erreur de saisie (message) ; `202` + `approval_request` → toast « demande d'approbation envoyée » et lien vers `/approvals/{id}` ; `202` + `task` → suivre la tâche (F5-03).
- **Permissions** : `GET /teams/{team}` renvoie les permissions de l'appelant sur l'équipe ; `GET /teams/{team}/permissions?scope_type=&scope_id=` pour une portée précise (projet, environnement, serveur, ressource). Masquer ou désactiver les actions non permises (le backend refuse de toute façon).
- **Temps réel** : WebSocket sur la même origine (`ws(s)://<hôte>/api/v1/...`). En développement avec Vite, mettre `NODRA_ALLOWED_ORIGINS=localhost:5173` côté backend et proxifier `/api` avec `ws: true`. Messages texte JSON `{type, ...}`, messages binaires = octets bruts (logs suivis, terminal) à décoder avec `TextDecoder` en mode flux.
- **Secrets** : jamais réaffichés par l'API (`********`) ; renvoyer `********` tel quel conserve la valeur existante.
- **Identifiants** : UUID partout ; les URL du front peuvent les reprendre (`/t/{team}/apps/{id}`).
- **Langue** : interface en français (tous les messages du backend le sont déjà).

---

## Lot 0 — Socle

### F0-01 · Initialiser `web/` `S`
- Vite + React + TypeScript strict, ESLint + Prettier, alias `@/`, structure `src/{api,components,features,routes,lib}`.
- `npm run build` doit produire `web/dist` (embarqué par Go via `web/embed.go`, déjà construit par le `Dockerfile` si `web/package.json` existe).
- Proxy Vite : `/api` → `http://localhost:8000` (avec `ws: true`).
- **Fini quand** : `npm run dev` affiche une page, `npm run build` puis `go run ./cmd/nodra serve` sert le front sur `:8000`, et `docker build .` réussit avec le front.

### F0-02 · Design system `M`
- Tailwind + shadcn/ui, thème clair/sombre (préférence système + bascule mémorisée), palette et typographie.
- Composants de base : Button, Input, Textarea, Select, Switch, Checkbox, Dialog, Drawer, Toast, Table (tri, pagination), Tabs, Badge de statut (running/stopped/exited/degraded/queued/in_progress/finished/failed/cancelled), Skeleton, état vide, ConfirmDialog (confirmation en tapant le nom pour les suppressions), champ secret (masqué/afficher/copier), bouton copier.
- **Fini quand** : une page « catalogue » interne montre tous les composants dans les deux thèmes.

### F0-03 · Client API typé `M`
- `api.ts` : wrapper `fetch` (base `/api/v1`, cookies, `X-Requested-With`, JSON, erreurs typées `ApiError {status, message}`).
- Types TypeScript des objets (`User`, `Team`, `Server`, `Application`, `Database`, `Service`, `Deployment`, `ServerTask`, `ApprovalRequest`, `AuditEvent`…) d'après API.md.
- TanStack Query : clés par équipe (`["team", teamId, "applications"]`), invalidations après mutations, gestion globale des `401`/`403`/`202 approval_request`.
- **Fini quand** : un hook `useMe()` et un hook `useServers(team)` fonctionnent, et les erreurs s'affichent en toast.

### F0-04 · Routage et mise en page `M`
- TanStack Router (ou React Router) : zone publique (`/login`, `/register`, `/forgot-password`, `/reset-password`, `/verify-email`, `/invitations/accept`, `/login/2fa`) et zone connectée sous `/t/{team}/…`.
- Garde d'authentification (`GET /me`), redirection avec `redirect=`.
- Mise en page : barre latérale (Tableau de bord, Projets, Serveurs, Sources, Stockages S3, Registres, Notifications, Équipe, Paramètres), en-tête (sélecteur d'équipe F3-01, compte, badge d'approbations en attente).
- **Fini quand** : navigation entre pages vides, équipe courante conservée dans l'URL, retour à la page demandée après connexion.

### F0-05 · WebSocket `M`
- Hook `useSocket(url, {onText, onBinary})` : reconnexion avec délai croissant, fermeture propre, état (connecté/reconnexion).
- Hook `useTeamEvents(team)` sur `GET /teams/{team}/events` : à chaque `{type: "event", event: {kind, id, resource, server, status}}`, invalider les requêtes concernées (`resource.status`, `deployment.status`, `deployment.log`, `task.status`, `task.output`, `server.status`, `proxy.status`).
- **Fini quand** : un changement de statut d'une ressource (démarrage d'une base) se reflète sans recharger la page.

### F0-06 · Utilitaires `S`
- Dates relatives (« il y a 3 min ») et absolues, tailles (`Kio`, `Mio`), durées, copie dans le presse-papiers, téléchargement de fichier (texte et lien présigné).
- **Fini quand** : utilisés par au moins une page.

### F0-07 · Permissions dans l'interface `S`
- Hook `usePermission(perm, scope?)` (cache par portée) et composant `<Can perm="resource.deploy" scope={...}>`.
- Catalogue `GET /permissions` (racine) : libellés et catégories des permissions, types de portée, scopes de jetons, actions soumises à approbation.
- **Fini quand** : un lecteur (rôle viewer) ne voit pas les boutons de déploiement.

## Lot 1 — Authentification

### F1-01 · Connexion `M`
- `GET /instance` (nom de l'instance, `registration_enabled`, `oauth_providers`), `POST /auth/login` → si `mfa_required`, aller sur `/login/2fa`.
- `/login/2fa` : `POST /auth/login/2fa` avec `{code}` ou `{recovery_code}` (lien « utiliser un code de secours »).
- Boutons OAuth : liens vers `/api/v1/auth/oauth/{provider}/start?redirect=<page>` ; afficher `?oauth_error=` s'il est présent.
- **Fini quand** : connexion par mot de passe, avec 2FA et par un fournisseur OAuth configuré ; compte désactivé (`403`) affiché proprement.

### F1-02 · Inscription `S`
- `POST /auth/register {email, password, name?}` ; lien masqué si `registration_enabled` est faux (le tout premier compte reste possible : il devient administrateur de l'instance).
- **Fini quand** : création de compte puis arrivée sur le tableau de bord de l'équipe personnelle.

### F1-03 · Mot de passe oublié `S`
- `POST /auth/password/forgot {email}` (message identique que le compte existe ou non), page `/reset-password?token=` → `POST /auth/password/reset {token, password}`.
- **Fini quand** : parcours complet avec l'e-mail reçu (instance avec e-mail configuré, F12-02).

### F1-04 · Vérification d'adresse `S`
- Page `/verify-email?token=` → `POST /auth/verify-email {token}` ; bandeau « adresse non vérifiée » si `user.email_verified_at` est vide, avec « renvoyer » → `POST /me/verify-email`.
- **Fini quand** : le bandeau disparaît après vérification.

### F1-05 · Acceptation d'invitation `S`
- Page `/invitations/accept?token=` → `POST /invitations/accept {token}` ; si non connecté, passer par la connexion ou l'inscription puis revenir ; `403` si l'e-mail ne correspond pas.
- **Fini quand** : l'équipe apparaît dans le sélecteur après acceptation.

### F1-06 · Déconnexion `S`
- `POST /auth/logout`, vider le cache TanStack Query.
- **Fini quand** : retour à `/login`, aucune donnée en cache.

## Lot 2 — Compte

### F2-01 · Profil et mot de passe `S`
- `GET /me`, `PATCH /me {name}`, `POST /me/password {current_password, new_password}` (les autres sessions sont révoquées). Comptes créés par OAuth : pas de mot de passe actuel → proposer « mot de passe oublié ».
- **Fini quand** : modifications enregistrées avec toasts.

### F2-02 · Double authentification `M`
- `POST /me/2fa/setup` → QR code depuis `otpauth_url` (lib `qrcode`) + secret ; `POST /me/2fa/enable {code}` → afficher et faire télécharger les 8 codes de secours (une seule fois) ; `POST /me/2fa/disable {password}`.
- **Fini quand** : activation, connexion avec code, désactivation.

### F2-03 · Sessions `S`
- `GET /me/sessions` (IP, navigateur, dernière activité, session courante), `DELETE /me/sessions/{id}`.

### F2-04 · Jetons API `M`
- `GET/POST/DELETE /me/tokens` : nom, équipe (`team_id`), scopes (`read`, `write`, `deploy`, `root`), expiration en jours ; le jeton n'est affiché qu'à la création (copier).
- Aide : exemple `curl` et configuration MCP (voir API.md, « Serveur MCP »).
- **Fini quand** : création, copie, révocation.

### F2-05 · Comptes liés `S`
- `GET /me/identities`, lier : lien `/api/v1/auth/oauth/{provider}/start?link=1&redirect=/compte`, retour avec `?oauth_linked=` ou `?oauth_error=` ; `DELETE /me/identities/{provider}` (`409` si dernier moyen de connexion).

## Lot 3 — Équipes et accès

### F3-01 · Sélecteur d'équipe `M`
- `GET /teams` (équipes et sous-équipes, organisation en arborescence), `POST /teams {name, parent_id?}`.
- **Fini quand** : changement d'équipe dans l'en-tête, création d'organisation et de sous-équipe.

### F3-02 · Paramètres d'équipe `M`
- `GET/PATCH /teams/{team}`, `POST /leave`, `POST /transfer-ownership {user_id}`, `DELETE /teams/{team}` : si `409 {resources}`, proposer de recommencer avec `{"delete_resources": true}` (confirmation forte : arrêt et effacement des serveurs, volumes compris).
- **Fini quand** : suppression d'équipe avec ressources possible après confirmation explicite.

### F3-03 · Membres et invitations `M`
- `GET /members` (directs et hérités, rôles), `POST /members {user_id, role_id}` (sous-équipe : membre de l'organisation parente), `DELETE /members/{user}`.
- `GET/POST/DELETE /invitations` (e-mail, rôle, lien d'invitation à copier).

### F3-04 · Rôles personnalisés `L`
- `GET/POST/PATCH/DELETE /roles` ; éditeur en matrice : permissions groupées par catégorie (catalogue `GET /permissions`), jokers (`resource.*`), rôles système en lecture seule.
- **Fini quand** : création d'un rôle « Déployeur » (lecture + `resource.deploy`) attribuable.

### F3-05 · Attributions de droits `L`
- `GET/POST/DELETE /role-assignments` : utilisateur ou groupe × rôle × portée (équipe, projet, environnement, serveur, ressource) × allow/deny.
- Sélecteur de portée réutilisable (projet → environnement → ressource, ou serveur).
- Vue « qui a accès » sur la page d'une ressource (onglet Accès).
- **Fini quand** : un refus sur une application précise cache cette application au membre concerné.

### F3-06 · Groupes `S`
- `GET/POST/PATCH/DELETE /groups`, `PUT/DELETE /groups/{id}/members/{user}`.

### F3-07 · Journal d'audit `M`
- `GET /audit?actor=&action=&target_type=&target_id=&since=&until=&before=&limit=` : tableau filtrable, pagination par `next_before`, détail des métadonnées (JSON), export `GET /audit/export` (CSV).

### F3-08 · Approbations `L`
- Règles : `GET/POST/PATCH/DELETE /approval-rules` (action soumise, portée, nombre d'approbateurs, qui peut approuver).
- File : `GET /approvals` (en attente / traitées), détail `GET /approvals/{id}` (résumé, demandeur, votes), `POST /approvals/{id}/approve|reject|cancel`.
- Badge d'en-tête (nombre en attente), lien depuis les toasts `202 approval_request`.
- **Fini quand** : déploiement en production bloqué puis exécuté après approbation par un second membre.

## Lot 4 — Tableau de bord

### F4-01 · Vue d'ensemble `M`
- Ressources `GET /resources` (statuts, regroupées par projet), serveurs `GET /servers` (joignable, proxy), déploiements récents `GET /deployments?limit=10`, approbations en attente.
- Mise à jour temps réel (F0-05).

### F4-02 · Recherche rapide `S`
- Palette de commandes (Ctrl+K) : aller à une ressource, un serveur, un projet (données de `GET /resources`, `GET /servers`, `GET /projects`).

## Lot 5 — Serveurs

### F5-01 · Liste et ajout de serveur `L`
- `GET /servers` ; assistant d'ajout : clé SSH (`GET /private-keys`, `POST /private-keys` vide = génération, ou import ; afficher la clé publique à ajouter dans `authorized_keys`), nom, IP, port, utilisateur, type de proxy → `POST /servers` (renvoie `task` de validation à suivre, F5-03).
- Écran « Clés SSH » : `GET/PATCH/DELETE /private-keys/{id}` (`409` si utilisée).
- **Fini quand** : un serveur ajouté passe à « utilisable » en suivant la validation en direct.

### F5-02 · Détail d'un serveur `L`
- `GET /servers/{id}` (infos, destinations), actions : `validate`, `install-docker`, `docker-cleanup`, `packages/check`, `packages/update`, `reboot` (toutes renvoient une tâche).
- Métriques `GET /servers/{id}/metrics?since=` : graphiques CPU, mémoire, disque, charge (Recharts ou équivalent), plage 1 h / 24 h / 7 j.
- Ressources et domaines hébergés : `GET /servers/{id}/resources`, `GET /servers/{id}/domains`.

### F5-03 · Tâches serveur `M`
- `GET /servers/{id}/tasks`, détail `GET /servers/{id}/tasks/{task}` ; sortie en direct `WS /servers/{id}/tasks/{task}/stream` (`output`, `reset`, `status`, `end`).
- Annulation `POST /tasks/{task}/cancel` (`204` en file, `202 cancelling` en cours, `409` terminée).
- Composant réutilisable `<TaskOutput>` (utilisé par les actions des bases, services, sauvegardes, log drains…).

### F5-04 · Terminal du serveur `M`
- xterm.js + addon fit sur `WS /servers/{id}/terminal?rows=&cols=` : saisie en binaire, `{type: "resize"}` au redimensionnement, `{type: "exit"}` en fin ; plein écran.
- **Fini quand** : commandes interactives (`top`, `vim`) utilisables.

### F5-05 · Proxy `L`
- `GET/PATCH /servers/{id}/proxy` (type, e-mail ACME, compose personnalisé), `start` / `stop` / `restart`.
- Configurations dynamiques `GET/PUT/DELETE /proxy/configs/{name}` (éditeur CodeMirror YAML ou Caddyfile).
- Certificats personnalisés `GET/POST/DELETE /proxy/certificates`.
- Logs du proxy en direct `WS /servers/{id}/proxy/logs/stream?tail=`.

### F5-06 · Destinations `S`
- `GET/POST/DELETE /servers/{id}/destinations` (réseaux Docker).

### F5-07 · Paramètres du serveur `M`
- `PATCH /servers/{id}` (réglages : builds simultanés, délai de déploiement, nettoyage Docker : fréquence, seuil, volumes/réseaux ; domaine joker ; métriques).
- Cloudflare Tunnel : `PUT/DELETE /servers/{id}/cloudflared {token}` ; réinitialisation de la clé d'hôte `POST /host-key/reset` ; suppression `DELETE /servers/{id}` (approbations).

### F5-08 · Log drain `S`
- `GET/PUT/DELETE /servers/{id}/log-drain` : formulaire selon le type (New Relic, Axiom, personnalisé avec éditeur de configuration Fluent Bit) ; suivi de la tâche.

### F5-09 · Autorité de certification `S`
- `GET /servers/{id}/ca-certificate` : téléchargement du certificat (`.crt`) pour les clients TLS des bases.

### F5-10 · Commande ponctuelle `S`
- `POST /servers/{id}/commands {command}` puis sortie en direct (F5-03).

## Lot 6 — Projets et environnements

### F6-01 · Projets et environnements `M`
- `GET/POST/PATCH/DELETE /projects`, environnements `POST/PATCH/DELETE /projects/{p}/environments/{e}`, clonage `POST .../clone {name, project_id?, with_resources?}`. Suppressions soumises aux approbations.

### F6-02 · Vue d'un environnement `M`
- `GET /resources?environment_id=` (cartes par ressource avec statut, domaines, tags), filtres type/tag, bouton « Nouvelle ressource » (choix : application, base, service).

### F6-03 · Variables partagées `M`
- `GET/POST/PATCH/DELETE /variables` par portée (équipe, projet, environnement, serveur), secrets jamais réaffichés ; aide sur la syntaxe `{{team.NOM}}`, `{{project.NOM}}`, `{{environment.NOM}}`, `{{server.NOM}}`.

### F6-04 · Tags `S`
- `GET/POST/PATCH/DELETE /tags` (couleur), filtres par tag dans les listes.

## Lot 7 — Sources Git

### F7-01 · Sources `M`
- `GET/POST/PATCH/DELETE /sources` : GitHub App (manuelle), GitLab et Gitea par jeton.

### F7-02 · Création automatique d'une GitHub App `M`
- `POST /sources/github/manifest {name, organization?, html_url?, is_public?}` → le front soumet un formulaire `POST` vers `form_action` avec `manifest` ; GitHub revient sur `/api/v1/sources/github/callback` puis l'installation.
- **Fini quand** : App créée et installée, dépôts listés.

### F7-03 · Sélecteurs dépôt et branche `S`
- `GET /sources/{id}/repositories`, `GET /sources/{id}/branches?repo=` (composants réutilisés par F8-01).

## Lot 8 — Applications

### F8-01 · Assistant de création `L`
- Étapes : origine (dépôt public, source Git, clé de déploiement, image Docker, compose collé), dépôt/branche (F7-03), build pack (`nixpacks`, `dockerfile`, `dockercompose`, `dockerimage`, `static`), destination (serveur + réseau) → `POST /applications`.
- **Fini quand** : création d'une application nixpacks depuis un dépôt public avec domaine généré.

### F8-02 · Onglet Général `L`
- `GET/PATCH /applications/{id}` : nom, description, build pack, ports exposés et publiés, commandes d'installation/build/démarrage, dossier de base, Dockerfile, vérification de santé, limites (CPU, mémoire), réglages (`settings`), redirection www, authentification HTTP (mot de passe jamais réaffiché : `basic_auth_password_set`).
- Domaines : liste éditable + vérification DNS `POST /dns-check {domain, server_id}` (afficher `resolved`, `expected`, `message`).

### F8-03 · Onglet Variables `M`
- `GET/POST/PATCH/DELETE /applications/{id}/envs`, import `.env` `POST /envs/bulk {content}` ; options : build/runtime, preview, littérale, multiligne, secret ; modifications soumises aux approbations si une règle `resource.env.write` existe.

### F8-04 · Onglet Stockages `S`
- `GET/POST/DELETE /applications/{id}/storages` : volume, bind, fichier (éditeur de contenu).

### F8-05 · Onglet Déploiements `L`
- `GET /applications/{id}/deployments`, déclencher `POST /deploy {force_rebuild?, commit_sha?}`, `restart`, `stop`, retour arrière `GET /images` + `POST /rollback {tag}`, annulation `POST /deployments/{dep}/cancel`.
- Détail : logs en direct `WS /applications/{id}/deployments/{dep}/logs/stream?debug=1` (`logs`, `status`, `end`), coloration par type (commande, stdout, stderr), bascule « logs détaillés », défilement automatique.
- **Fini quand** : on suit un déploiement de bout en bout sans recharger.

### F8-06 · Onglet Logs `M`
- `GET /applications/{id}/containers`, logs suivis `WS /applications/{id}/logs/stream?tail=&container=` (binaire), recherche dans la sortie, pause, téléchargement.

### F8-07 · Onglet Terminal `S`
- `WS /applications/{id}/terminal?container=` (même composant que F5-04, choix du conteneur s'il y en a plusieurs).

### F8-08 · Onglet Previews `M`
- `GET/POST/PATCH/DELETE /applications/{id}/previews[/{pr}]`, `POST /previews/{pr}/deploy|stop` ; domaines (`domains`) et, pour Compose, `compose_domains` par service ; modèle d'URL `preview_url_template`.

### F8-09 · Onglet Webhooks et CI `S`
- `GET /applications/{id}/webhooks` (URLs GitHub/GitLab/Gitea/Bitbucket/CI + secret), `POST /webhooks/rotate` ; exemples `curl` pour `POST /teams/{team}/deploy?uuid=&tag=&force=true`.

### F8-10 · Onglet Tâches planifiées `M`
- `GET/POST/PATCH/DELETE /scheduled-tasks?resource_id=`, exécution `POST /scheduled-tasks/{id}/run`, historique `GET /executions` (sortie).

### F8-11 · Onglet Avancé `M`
- Labels personnalisés, options `docker run`, commandes pré/post-déploiement, log drain (`settings.log_drain_enabled`), tags `PUT /tags {tag_ids}`, déplacer `POST /move {environment_id}`, cloner `POST /clone {name?, environment_id?, destination_id?}`, supprimer `DELETE` (approbations, volumes en option).

### F8-12 · Applications Docker Compose `M`
- Éditeur YAML (CodeMirror) pour `docker_compose_raw`, emplacement du fichier dans le dépôt, domaines par service (`docker_compose_domains`), aperçu des services détectés.

## Lot 9 — Bases de données

### F9-01 · Création `M`
- Choix du type (PostgreSQL, MySQL, MariaDB, MongoDB, Redis, KeyDB, Dragonfly, ClickHouse) et de l'image, nom, identifiants (générés si vides), destination, `instant_deploy` → `POST /databases`.

### F9-02 · Page d'une base `L`
- `GET/PATCH /databases/{id}` : URL interne/publique (masquées sans `resource.env.read`, copier), mot de passe (afficher/copier, changement soumis à `resource.env.write`), exposition publique (`is_public`, `public_port`), configuration personnalisée, scripts d'initialisation, limites, vérification de santé.
- TLS : `settings.enable_ssl`, `settings.ssl_mode` (selon le type), téléchargement de `ca_certificate`.

### F9-03 · Actions, logs et terminal `M`
- `POST /databases/{id}/start|stop|restart` (tâche à suivre), logs suivis `WS /databases/{id}/logs/stream`, terminal `WS /databases/{id}/terminal`.

### F9-04 · Sauvegardes `L`
- Planifications `GET/POST/PATCH/DELETE /backups` (fréquence cron, rétention locale/S3, stockage S3, bases ciblées), exécution immédiate, historique `GET /backup-executions?…`, téléchargement `GET /backup-executions/{id}/download` (lien présigné), restauration `POST /backup-executions/{id}/restore` (approbations), suppression.

### F9-05 · Import de dump `M`
- `POST /databases/{id}/import?filename=&database=` avec le fichier en corps brut (streaming, barre de progression via `XMLHttpRequest.upload.onprogress`, en-tête `X-Requested-With`), puis suivi de la tâche.

### F9-06 · Variables, stockages, tâches, accès `S`
- Réutiliser les onglets génériques (F8-03, F8-04, F8-10, F3-05) sur `/databases/{id}/…`, cloner/déplacer/supprimer.

## Lot 10 — Services one-click

### F10-01 · Catalogue `M`
- `GET /service-templates?q=&category=` : grille avec logo, slogan, catégorie, recherche instantanée, filtres ; détail `GET /service-templates/{slug}` (documentation, compose).

### F10-02 · Création `M`
- `POST /services {template | docker_compose_raw, name?, environment_id, destination_id, instant_deploy?}` ; afficher les variables générées (mots de passe, URL) après création.

### F10-03 · Page d'un service `L`
- `GET/PATCH /services/{id}` (compose éditable, `log_drain_enabled`), sous-services `PATCH /services/{id}/apps/{nom}` (domaines, port, exclusion du statut), actions `start/stop/restart` et `apps/{nom}/restart`, tâches `GET /tasks`.

### F10-04 · Logs, terminal, variables `M`
- Logs `WS /services/{id}/logs/stream?app=`, terminal `WS /services/{id}/terminal?app=&container=`, variables, tags, cloner/déplacer/supprimer.

## Lot 11 — Intégrations d'équipe

### F11-01 · Stockages S3 `M`
- `GET/POST/PATCH/DELETE /s3-storages`, vérification `POST /s3-storages/{id}/check`.

### F11-02 · Notifications `L`
- Canaux `GET/POST/PATCH/DELETE /notification-channels` : formulaire par type (e-mail : destinataires ; Discord ; Slack ; Telegram : jeton du bot, chat, sujets par événement ; Pushover ; webhook), cases d'événements depuis `GET /notifications/catalog`, test `POST /notification-channels/{id}/test`, journal `GET /notification-deliveries`.

### F11-03 · Registres Docker privés `S`
- `GET/POST/PATCH/DELETE /docker-registries` (hôte, utilisateur, mot de passe ou jeton).

## Lot 12 — Administration de l'instance (visible si `is_instance_admin`)

### F12-01 · Réglages et mises à jour `M`
- `GET/PATCH /admin/settings` : domaine, nom, IP publiques, fuseau, inscriptions, validation DNS et serveurs DNS, accès API (activé, IP autorisées), serveur MCP, mises à jour automatiques (fréquences).
- Version : `version`, `update_available`, boutons « vérifier » `POST /admin/update-check` et « mettre à jour » `POST /admin/update` (tâche à suivre).

### F12-02 · E-mail de l'instance `S`
- `GET/PUT/DELETE /admin/mail` (SMTP ou Resend), test `POST /admin/mail/test {to?}`.

### F12-03 · Fournisseurs de connexion `S`
- `GET /admin/oauth` (avec `redirect_uri` à copier pour chaque fournisseur), `PUT /admin/oauth/{provider}`.

### F12-04 · Utilisateurs `M`
- `GET /admin/users?search=`, `PATCH /admin/users/{id} {is_instance_admin?, disabled?}`, `POST /admin/users/{id}/logout`, `DELETE /admin/users/{id}`.

### F12-05 · Audit de l'instance `S`
- `GET /admin/audit?scope=instance|all&…` (même composant que F3-07).

## Lot 13 — Transverse et qualité

### F13-01 · Temps réel partout `M`
- Brancher `useTeamEvents` (F0-05) sur toutes les listes et pages de détail ; toasts discrets « déploiement terminé / échoué ».

### F13-02 · États et confirmations `M`
- États vides utiles (avec action), chargements (skeletons), erreurs ; confirmations des actions destructives (nom à taper) ; messages des `202` (tâche ou approbation).

### F13-03 · Accessibilité et responsive `M`
- Navigation clavier, focus visibles, contrastes AA, libellés ARIA ; consultation confortable sur mobile (listes, statuts, logs).

### F13-04 · Tests `L`
- Vitest + Testing Library (client API, hooks, composants clés) ; Playwright sur une instance de test (`docker compose -f docker-compose.dev.yml up -d` + `go run ./cmd/nodra serve`) : inscription, ajout de serveur, déploiement, lecteur sans droit de déploiement, approbation en production.

### F13-05 · Intégration continue du front `S`
- Ajouter au workflow `.github/workflows/ci.yml` : `npm ci`, lint, tests, `npm run build` dans `web/` ; vérifier que l'image Docker embarque le front.

---

**Ordre conseillé** : Lot 0 → Lot 1 → F3-01/F4-01 → Lot 5 (serveurs) → Lot 6 → Lot 8 (applications) → Lot 9 → Lot 10 → le reste. Les composants communs (F5-03 sortie de tâche, F5-04 terminal, F8-03/F8-04 variables et stockages, F3-05 sélecteur de portée) se réutilisent dans plusieurs lots : les faire une fois, bien.
