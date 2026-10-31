package deploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/git"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// AppsDir : dossier des applications sur les serveurs.
const AppsDir = "/data/nodra/applications"

var ErrCancelled = errors.New("déploiement annulé")

type Runner struct {
	Store   *store.Store
	Box     *crypto.Box
	SSH     *ssh.Pool
	Servers *servers.Service
	Log     *slog.Logger
	AppsDir string // dossier des applications sur les serveurs (AppsDir par défaut)
	// DatabasesDir : dossier des bases sur les serveurs (DatabasesDir par défaut).
	DatabasesDir string
	// ServicesDir : dossier des services sur les serveurs (ServicesDir par défaut).
	ServicesDir string
	// OnFinish est appelé à la fin de chaque déploiement (commentaires de PR, notifications).
	OnFinish func(ctx context.Context, d *store.Deployment)
}

func (r *Runner) appsDir() string {
	if r.AppsDir != "" {
		return r.AppsDir
	}
	return AppsDir
}

// SourceFor déchiffre les identifiants d'une source Git.
func SourceFor(box *crypto.Box, g *store.GitSource) (git.Source, error) {
	src := git.Source{ID: g.ID, Type: g.Type, APIURL: strings.TrimSuffix(g.APIURL, "/"), HTMLURL: g.HTMLURL,
		AppID: g.AppID, InstallationID: g.InstallationID}
	var err error
	if g.PrivateKeyEnc != "" {
		if src.PrivateKey, err = box.Decrypt(g.PrivateKeyEnc); err != nil {
			return src, err
		}
	}
	if g.AccessTokenEnc != "" {
		if src.AccessToken, err = box.Decrypt(g.AccessTokenEnc); err != nil {
			return src, err
		}
	}
	return src, nil
}

// --- Journal du déploiement ---

type depLog struct {
	st      *store.Store
	id      string
	mu      sync.Mutex
	pending []store.DeploymentLog
	partial map[string]string
	secrets []string
	stop    chan struct{}
	done    chan struct{}
}

func newDepLog(st *store.Store, id string) *depLog {
	l := &depLog{st: st, id: id, partial: map[string]string{}, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(l.done)
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-l.stop:
				l.flush()
				return
			case <-t.C:
				l.flush()
			}
		}
	}()
	return l
}

func (l *depLog) addSecret(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(s) >= 4 {
		l.secrets = append(l.secrets, s)
	}
}

func (l *depLog) line(kind string, hidden bool, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, store.DeploymentLog{CreatedAt: time.Now(), Kind: kind, Hidden: hidden, Message: Redact(msg, l.secrets)})
}

func (l *depLog) flush() {
	l.mu.Lock()
	lines := l.pending
	l.pending = nil
	l.mu.Unlock()
	if err := l.st.AppendDeploymentLogs(context.Background(), l.id, lines); err != nil {
		slog.Default().Error("journal de déploiement", "err", err)
	}
}

func (l *depLog) Close() {
	l.mu.Lock()
	for k, rest := range l.partial {
		if rest != "" {
			kind, hidden := k[:len(k)-1], k[len(k)-1] == 'h'
			l.pending = append(l.pending, store.DeploymentLog{CreatedAt: time.Now(), Kind: kind, Hidden: hidden, Message: Redact(rest, l.secrets)})
		}
	}
	l.partial = map[string]string{}
	l.mu.Unlock()
	close(l.stop)
	<-l.done
}

type logWriter struct {
	l      *depLog
	kind   string
	hidden bool
}

func (w logWriter) Write(p []byte) (int, error) {
	key := w.kind + map[bool]string{true: "h", false: "v"}[w.hidden]
	w.l.mu.Lock()
	buf := w.l.partial[key] + string(p)
	lines := strings.Split(buf, "\n")
	w.l.partial[key] = lines[len(lines)-1]
	w.l.mu.Unlock()
	for _, ln := range lines[:len(lines)-1] {
		w.l.line(w.kind, w.hidden, strings.TrimRight(ln, "\r"))
	}
	return len(p), nil
}

// --- Exécution ---

type pipe struct {
	r             *Runner
	d             *store.Deployment
	a             *store.Application
	sv            *store.Server
	t             ssh.Target
	loc           *store.ResourceLocation
	log           *depLog
	appDir        string
	workdir       string
	src           string
	env           Env
	domains       []string
	image         string
	containerName string
	rolling       bool
	certs         map[string][2]string
	dockerEnv     string // DOCKER_CONFIG des registres privés de l'équipe
}

func (p *pipe) info(format string, args ...any) {
	p.log.line("info", false, fmt.Sprintf(format, args...))
}
func (p *pipe) debug(format string, args ...any) {
	p.log.line("info", true, fmt.Sprintf(format, args...))
}

// sh exécute un script (passé par stdin, absent de la liste des processus du serveur).
func (p *pipe) sh(ctx context.Context, label, script string, hidden bool) error {
	if label != "" {
		p.log.line("command", hidden, label)
	}
	code, err := p.r.SSH.RunInput(ctx, p.t, "sh -s", strings.NewReader("set -e\n"+p.dockerEnv+script),
		logWriter{p.log, "stdout", hidden}, logWriter{p.log, "stderr", hidden})
	if err != nil {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s : échec (code %d)", strings.TrimPrefix(label, "$ "), code)
	}
	return nil
}

// out exécute un script et renvoie sa sortie standard (non journalisée).
func (p *pipe) out(ctx context.Context, script string) (string, error) {
	var stdout, stderr bytes.Buffer
	code, err := p.r.SSH.RunInput(ctx, p.t, "sh -s", strings.NewReader(script), &stdout, &stderr)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return stdout.String(), fmt.Errorf("code %d : %s", code, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// writeFile écrit un fichier sur le serveur.
func writeFile(path, content string, mode string) string {
	return fmt.Sprintf("mkdir -p %s\nprintf %%s %s | base64 -d > %s\nchmod %s %s\n", ssh.Quote(path[:strings.LastIndex(path, "/")]),
		ssh.Quote(base64.StdEncoding.EncodeToString([]byte(content))), ssh.Quote(path), mode, ssh.Quote(path))
}

// Run exécute un déploiement déjà réservé (statut in_progress).
func (r *Runner) Run(ctx context.Context, deploymentID string) error {
	d, err := r.Store.DeploymentByID(ctx, deploymentID)
	if err != nil {
		return err
	}
	log := newDepLog(r.Store, d.ID)
	p := &pipe{r: r, d: d, log: log}
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { // surveillance de la demande d'annulation
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if c, _ := r.Store.CancelRequested(context.Background(), d.ID); c {
					cancel()
					return
				}
			}
		}
	}()
	runErr := p.run(ctx)
	status, msg := "finished", ""
	switch {
	case runErr == nil:
		p.info("Déploiement terminé.")
	case errors.Is(parent.Err(), context.DeadlineExceeded):
		status, msg = "failed", "délai de déploiement dépassé (réglage deployment_timeout du serveur)"
		log.line("error", false, "Échec : "+msg)
	case errors.Is(runErr, ErrCancelled) || ctx.Err() != nil:
		status, msg = "cancelled", "annulé"
		p.info("Déploiement annulé.")
	default:
		status, msg = "failed", Redact(runErr.Error(), log.secrets)
		log.line("error", false, "Échec : "+msg)
	}
	if p.workdir != "" && !(p.a != nil && p.a.Settings.PreserveRepository) {
		_, _ = p.out(context.WithoutCancel(ctx), "rm -rf "+ssh.Quote(p.workdir))
	}
	log.Close()
	bg := context.WithoutCancel(ctx)
	if status == "finished" && p.a != nil {
		if d.PRNumber == 0 {
			_ = r.Store.SetApplicationStatus(bg, p.a.ID, "running")
		} else {
			_ = r.Store.SetPreviewStatus(bg, p.a.ID, d.PRNumber, "running")
		}
	}
	err = r.Store.FinishDeployment(bg, d.ID, status, p.image, msg)
	if r.OnFinish != nil {
		d.Status, d.Image, d.Error = status, p.image, msg
		r.OnFinish(bg, d)
	}
	return err
}

func (p *pipe) run(ctx context.Context) error {
	var err error
	r, d := p.r, p.d
	if p.a, err = r.Store.ApplicationByIDAny(ctx, d.ApplicationID); err != nil {
		return err
	}
	a := p.a
	if p.loc, err = r.Store.ResourceLocation(ctx, a.ID); err != nil {
		return err
	}
	if p.sv, err = r.Store.ServerByIDAny(ctx, p.loc.ServerID); err != nil {
		return err
	}
	if !p.sv.IsUsable {
		return errors.New("le serveur n'est pas validé")
	}
	if p.t, err = r.Servers.Target(ctx, p.sv); err != nil {
		return err
	}
	if err := p.registries(ctx); err != nil {
		return err
	}
	p.appDir = r.appsDir() + "/" + a.ID
	p.workdir = p.appDir + "/deployments/" + d.ID
	p.src = p.workdir + "/src"
	p.info("Déploiement de %s sur %s (%s).", a.Name, p.sv.Name, p.sv.IP)

	p.domains = a.Domains
	if d.PRNumber > 0 {
		if pv, err := r.Store.PreviewByNumber(ctx, a.ID, d.PRNumber); err == nil {
			p.domains = pv.Domains
		}
	}
	consistent := a.Settings.ConsistentContainerName || d.PRNumber > 0
	switch {
	case d.PRNumber > 0:
		p.containerName = fmt.Sprintf("%s-pr-%d", a.ID, d.PRNumber)
	case consistent:
		p.containerName = a.ID
	default:
		p.containerName = a.ID + "-" + strings.ReplaceAll(d.ID, "-", "")[:8]
	}
	p.rolling = !(a.PortsMappings != "" || consistent || strings.Contains(a.CustomDockerRunOptions, "--ip"))
	if err := p.loadCerts(ctx); err != nil {
		return err
	}
	if err := p.resolveEnv(ctx); err != nil {
		return err
	}

	switch {
	case d.RollbackImage != "":
		p.image = d.RollbackImage
		p.info("Redéploiement de l'image existante %s.", p.image)
		if ok, _ := p.imageExists(ctx, p.image); !ok {
			return fmt.Errorf("image %s introuvable sur le serveur", p.image)
		}
	case a.BuildPack == "dockerimage":
		p.image = a.DockerImage
		if !strings.Contains(path.Base(a.DockerImage), ":") && !strings.Contains(a.DockerImage, "@") {
			tag := a.DockerImageTag
			if tag == "" {
				tag = "latest"
			}
			p.image += ":" + tag
		}
		if err := p.sh(ctx, "$ docker pull "+p.image, "docker pull "+ssh.Quote(p.image)+"\n", false); err != nil {
			return err
		}
	case a.BuildPack == "dockercompose":
		return p.deployCompose(ctx)
	default:
		if err := p.build(ctx); err != nil {
			return err
		}
	}
	return p.startContainer(ctx)
}

func (p *pipe) loadCerts(ctx context.Context) error {
	if p.sv.Proxy.Type != "caddy" {
		return nil
	}
	certs, err := p.r.Store.ListCertificates(ctx, p.sv.ID)
	if err != nil {
		return err
	}
	p.certs = map[string][2]string{}
	for _, c := range certs {
		for _, d := range c.Domains {
			p.certs[d] = [2]string{"/certs/" + c.ID + ".crt", "/certs/" + c.ID + ".key"}
		}
	}
	return nil
}

func (p *pipe) resolveEnv(ctx context.Context) error {
	r, a := p.r, p.a
	list, err := r.Store.ListEnvVars(ctx, a.ID)
	if err != nil {
		return err
	}
	vars := make([]Var, 0, len(list))
	for _, v := range list {
		val, err := r.Box.Decrypt(v.ValueEnc)
		if err != nil {
			return err
		}
		vars = append(vars, Var{Key: v.Key, Value: val, Buildtime: v.IsBuildtime, Runtime: v.IsRuntime, Preview: v.IsPreview,
			Literal: v.IsLiteral, Secret: v.IsSecret || a.Settings.RedactEnvValuesInLogs})
	}
	raw, err := r.Store.SharedVariablesFor(ctx, a.TeamID, p.loc.ProjectID, p.loc.EnvironmentID, p.loc.ServerID)
	if err != nil {
		return err
	}
	shared := make(map[string]string, len(raw))
	for k, v := range raw {
		if shared[k], err = r.Box.Decrypt(v.ValueEnc); err != nil {
			return err
		}
		if v.IsSecret {
			p.log.addSecret(shared[k])
		}
	}
	p.env = BuildEnv(vars, shared, p.d.PRNumber > 0, Defaults{ResourceID: a.ID, ContainerName: p.containerName, Branch: p.branch(),
		Commit: p.d.CommitSHA, Port: firstPort(a.PortsExposes), Domains: p.domains, Nixpacks: a.BuildPack == "nixpacks",
		IncludeInBuild: a.Settings.IncludeDefaultVariables, CommitInBuild: a.Settings.IncludeSourceCommit})
	for _, s := range p.env.Secrets {
		p.log.addSecret(s)
	}
	for _, m := range p.env.Missing {
		p.info("Attention : variable partagée {{%s}} introuvable (remplacée par une valeur vide).", m)
	}
	return nil
}

func (p *pipe) branch() string {
	if p.d.Branch != "" {
		return p.d.Branch
	}
	return p.a.GitBranch
}

func (p *pipe) imageExists(ctx context.Context, image string) (bool, error) {
	out, err := p.out(ctx, "docker image inspect "+ssh.Quote(image)+" >/dev/null 2>&1 && echo oui || echo non\n")
	return strings.TrimSpace(out) == "oui", err
}

// --- Récupération du code ---

// repoAccess renvoie l'URL de clonage et le préambule shell (clé de déploiement) ; les secrets sont masqués des logs.
func (p *pipe) repoAccess(ctx context.Context) (url, preamble string, err error) {
	a := p.a
	switch {
	case a.SourceID != nil:
		g, err := p.r.Store.GitSourceByID(ctx, a.TeamID, *a.SourceID)
		if err != nil {
			return "", "", fmt.Errorf("source Git : %w", err)
		}
		src, err := SourceFor(p.r.Box, g)
		if err != nil {
			return "", "", err
		}
		u, err := git.CloneURL(ctx, src, a.GitRepository)
		if err != nil {
			return "", "", fmt.Errorf("source Git : %w", err)
		}
		if i := strings.Index(u, "@"); i > 0 {
			p.log.addSecret(u[strings.Index(u, "://")+3 : i]) // identifiants de l'URL
		}
		return u, "export GIT_TERMINAL_PROMPT=0\n", nil
	case a.PrivateKeyID != nil:
		k, err := p.r.Store.PrivateKeyByID(ctx, a.TeamID, *a.PrivateKeyID)
		if err != nil {
			return "", "", fmt.Errorf("deploy key : %w", err)
		}
		pem, err := p.r.Box.Decrypt(k.PrivateKeyEnc)
		if err != nil {
			return "", "", err
		}
		keyFile := p.workdir + "/.deploy_key"
		pre := writeFile(keyFile, pem, "600") + "trap 'rm -f " + ssh.Quote(keyFile) + "' EXIT\n" +
			"export GIT_SSH_COMMAND=" + ssh.Quote("ssh -i "+keyFile+" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="+p.workdir+"/.known_hosts") + "\n"
		return a.GitRepository, pre, nil
	default:
		return a.GitRepository, "export GIT_TERMINAL_PROMPT=0\n", nil
	}
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// resolveCommit détermine le commit à déployer sans cloner (git ls-remote).
func (p *pipe) resolveCommit(ctx context.Context, url, pre string) (string, error) {
	if p.d.CommitSHA != "" {
		return p.d.CommitSHA, nil
	}
	if p.a.GitCommitSHA != "HEAD" {
		return p.a.GitCommitSHA, nil
	}
	out, err := p.out(ctx, "mkdir -p "+ssh.Quote(p.workdir)+"\n"+pre+"git ls-remote "+ssh.Quote(url)+" "+ssh.Quote("refs/heads/"+p.branch())+"\n")
	if err != nil {
		return "", fmt.Errorf("dépôt inaccessible : %s", Redact(err.Error(), p.log.secrets))
	}
	sha, _, _ := strings.Cut(strings.TrimSpace(out), "\t")
	if len(sha) < 7 {
		return "", fmt.Errorf("branche %s introuvable dans le dépôt", p.branch())
	}
	return sha, nil
}

func (p *pipe) clone(ctx context.Context, url, pre, sha string) error {
	a, st := p.a, p.a.Settings
	var b strings.Builder
	b.WriteString(pre)
	fmt.Fprintf(&b, "rm -rf %s\nmkdir -p %s\n", ssh.Quote(p.src), ssh.Quote(p.workdir))
	depth := ""
	if st.GitShallowClone {
		depth = " --depth 1"
	}
	fmt.Fprintf(&b, "git clone -q%s --branch %s %s %s\ncd %s\n", depth, ssh.Quote(p.branch()), ssh.Quote(url), ssh.Quote(p.src), ssh.Quote(p.src))
	if p.d.PRNumber > 0 {
		ref := fmt.Sprintf("pull/%d/head", p.d.PRNumber)
		if g, err := p.sourceType(ctx); err == nil && g == git.GitLab {
			ref = fmt.Sprintf("merge-requests/%d/head", p.d.PRNumber)
		}
		fmt.Fprintf(&b, "git fetch -q%s origin %s\ngit checkout -q FETCH_HEAD\n", depth, ssh.Quote(ref))
	} else {
		fmt.Fprintf(&b, "if [ \"$(git rev-parse HEAD)\" != %s ]; then git fetch -q%s origin %s && git checkout -q %s; fi\n",
			ssh.Quote(sha), depth, ssh.Quote(sha), ssh.Quote(sha))
	}
	if st.GitSubmodules {
		b.WriteString("[ -f .gitmodules ] && git submodule update -q --init --recursive" + depth + " || true\n")
	}
	if st.GitLFS {
		b.WriteString("if command -v git-lfs >/dev/null 2>&1; then git lfs pull >/dev/null 2>&1 || true; fi\n")
	}
	if err := p.sh(ctx, "$ git clone "+a.GitRepository+" (branche "+p.branch()+")", b.String(), false); err != nil {
		return err
	}
	out, err := p.out(ctx, "cd "+ssh.Quote(p.src)+" && git rev-parse HEAD && git log -1 --pretty=%s\n")
	if err != nil {
		return err
	}
	commit, msg, _ := strings.Cut(strings.TrimSpace(out), "\n")
	p.d.CommitSHA = commit
	p.info("Commit %s : %s", shortSHA(commit), msg)
	return p.r.Store.SetDeploymentCommit(ctx, p.d.ID, commit, msg)
}

func (p *pipe) sourceType(ctx context.Context) (string, error) {
	if p.a.SourceID == nil {
		return "", errors.New("aucune source")
	}
	g, err := p.r.Store.GitSourceByID(ctx, p.a.TeamID, *p.a.SourceID)
	if err != nil {
		return "", err
	}
	return g.Type, nil
}

// --- Construction ---

func (p *pipe) buildHash() string {
	a := p.a
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%t", a.BuildPack, a.BaseDirectory, a.Dockerfile, a.DockerfileLocation,
		a.DockerfileTarget, a.InstallCommand, a.BuildCommand, a.StartCommand, a.PublishDirectory, a.StaticImage, a.PortsExposes,
		a.WatchPaths, a.Settings.IsSPA)
	keys := make([]string, 0, len(p.env.Build))
	for k := range p.env.Build {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "|%s=%s", k, p.env.Build[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:8]
}

func (p *pipe) build(ctx context.Context) error {
	a := p.a
	inline := a.BuildPack == "dockerfile" && strings.TrimSpace(a.Dockerfile) != "" && a.GitRepository == ""
	sha := "inline"
	var url, pre string
	var err error
	if !inline {
		if url, pre, err = p.repoAccess(ctx); err != nil {
			return err
		}
		if sha, err = p.resolveCommit(ctx, url, pre); err != nil {
			return err
		}
		p.d.CommitSHA = sha
		p.env.Runtime["SOURCE_COMMIT"] = sha
	}
	tag := shortSHA(sha) + "-" + p.buildHash()
	if p.d.PRNumber > 0 {
		tag = fmt.Sprintf("pr%d-%s", p.d.PRNumber, tag)
	}
	p.image = "nodra/" + a.ID + ":" + tag
	if !p.d.ForceRebuild && !a.Settings.DisableBuildCache {
		if ok, _ := p.imageExists(ctx, p.image); ok {
			p.info("L'image %s existe déjà pour ce commit et cette configuration : construction ignorée.", p.image)
			return nil
		}
	}
	if inline {
		if err := p.sh(ctx, "", fmt.Sprintf("rm -rf %s\n", ssh.Quote(p.src))+writeFile(p.src+"/Dockerfile", a.Dockerfile, "644"), true); err != nil {
			return err
		}
	} else if err := p.clone(ctx, url, pre, sha); err != nil {
		return err
	}
	context := path.Join(p.src, a.BaseDirectory)
	labels := fmt.Sprintf(" --label %s --label nodra.resource=%s --label nodra.commit=%s", ManagedLabel, a.ID, shortSHA(p.d.CommitSHA))
	noCache := ""
	if p.d.ForceRebuild || a.Settings.DisableBuildCache {
		noCache = " --no-cache"
	}
	buildArgs := ""
	if a.Settings.InjectBuildArgs {
		keys := make([]string, 0, len(p.env.Build))
		for k := range p.env.Build {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buildArgs += " --build-arg " + ssh.Quote(k+"="+p.env.Build[k])
		}
	}
	target := ""
	if a.DockerfileTarget != "" {
		target = " --target " + ssh.Quote(a.DockerfileTarget)
	}
	switch a.BuildPack {
	case "dockerfile":
		file := path.Join(context, a.DockerfileLocation)
		if inline {
			file, context = p.src+"/Dockerfile", p.src
		}
		return p.sh(ctx, "$ docker build ("+a.BuildPack+")", fmt.Sprintf("cd %s\nDOCKER_BUILDKIT=1 docker build -f %s -t %s%s%s%s%s .\n",
			ssh.Quote(context), ssh.Quote(file), ssh.Quote(p.image), labels, target, noCache, buildArgs), false)
	case "nixpacks":
		return p.sh(ctx, "$ nixpacks build", p.nixpacksScript(context)+fmt.Sprintf(
			"DOCKER_BUILDKIT=1 docker build -f .nixpacks/Dockerfile -t %s%s%s%s .\n", ssh.Quote(p.image), labels, noCache, buildArgs), false)
	case "static":
		return p.buildStatic(ctx, context, labels, noCache, buildArgs)
	}
	return fmt.Errorf("build pack %s non pris en charge", a.BuildPack)
}

// nixpacksScript installe Nixpacks si besoin et génère le plan de build (.nixpacks/Dockerfile).
func (p *pipe) nixpacksScript(context string) string {
	a := p.a
	var b strings.Builder
	b.WriteString("command -v nixpacks >/dev/null 2>&1 || (curl -sSL https://nixpacks.com/install.sh | bash)\n")
	fmt.Fprintf(&b, "cd %s\nnixpacks build . --out .", ssh.Quote(context))
	for flag, v := range map[string]string{"--install-cmd": a.InstallCommand, "--build-cmd": a.BuildCommand, "--start-cmd": a.StartCommand} {
		if v != "" {
			b.WriteString(" " + flag + " " + ssh.Quote(v))
		}
	}
	keys := make([]string, 0, len(p.env.Build))
	for k := range p.env.Build {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(" --env " + ssh.Quote(k+"="+p.env.Build[k]))
	}
	b.WriteString("\n")
	return b.String()
}

func (p *pipe) buildStatic(ctx context.Context, context, labels, noCache, buildArgs string) error {
	a := p.a
	publish := strings.TrimPrefix(a.PublishDirectory, "/")
	tryFiles := "try_files $uri $uri/ =404;"
	if a.Settings.IsSPA {
		tryFiles = "try_files $uri $uri/ /index.html;"
	}
	nginx := "server {\n  listen 80;\n  root /usr/share/nginx/html;\n  index index.html index.htm;\n  gzip on;\n  location / {\n    " + tryFiles + "\n  }\n}\n"
	var dockerfile string
	script := writeFile(context+"/.nodra-nginx.conf", nginx, "644")
	if a.InstallCommand != "" || a.BuildCommand != "" {
		// Construction avec Nixpacks, puis copie du dossier publié dans l'image statique.
		builder := p.image + "-build"
		script += p.nixpacksScript(context) + fmt.Sprintf("DOCKER_BUILDKIT=1 docker build -f .nixpacks/Dockerfile -t %s%s%s .\n",
			ssh.Quote(builder), noCache, buildArgs)
		dockerfile = fmt.Sprintf("FROM %s AS build\nFROM %s\nCOPY --from=build /app/%s /usr/share/nginx/html\n", builder, a.StaticImage, publish)
	} else {
		dockerfile = fmt.Sprintf("FROM %s\nCOPY ./%s /usr/share/nginx/html\n", a.StaticImage, publish)
	}
	if strings.Contains(a.StaticImage, "nginx") {
		dockerfile += "COPY .nodra-nginx.conf /etc/nginx/conf.d/default.conf\n"
	}
	script += writeFile(context+"/.nodra-static.Dockerfile", dockerfile, "644") +
		fmt.Sprintf("cd %s\nDOCKER_BUILDKIT=1 docker build -f .nodra-static.Dockerfile -t %s%s .\n", ssh.Quote(context), ssh.Quote(p.image), labels)
	if a.InstallCommand != "" || a.BuildCommand != "" {
		script += "docker rmi " + ssh.Quote(p.image+"-build") + " >/dev/null 2>&1 || true\n"
	}
	return p.sh(ctx, "$ docker build (static)", script, false)
}

// --- Démarrage ---

func (p *pipe) writeStorageFiles(ctx context.Context) ([]*store.Storage, error) {
	storages, err := p.r.Store.ListStorages(ctx, p.a.ID)
	if err != nil {
		return nil, err
	}
	var script strings.Builder
	for _, s := range storages {
		if s.Kind == "file" {
			script.WriteString(writeFile(p.appDir+"/files/"+s.ID, s.Content, "644"))
		}
	}
	if script.Len() > 0 {
		if err := p.sh(ctx, "", script.String(), true); err != nil {
			return nil, err
		}
	}
	return storages, nil
}

func containersScript(appID string, pr int) string {
	return fmt.Sprintf("docker ps -aq --filter label=nodra.resource=%s --filter label=nodra.pr=%d", appID, pr)
}

// removeOld supprime les conteneurs de l'application (même PR), sauf keep.
func (p *pipe) removeOld(ctx context.Context, keep string) error {
	return p.sh(ctx, "$ arrêt des anciens conteneurs", fmt.Sprintf(`for c in $(%s); do
  name=$(docker inspect -f '{{.Name}}' "$c" | sed 's#^/##')
  [ "$name" = %s ] && continue
  echo "Arrêt de $name"
  docker stop -t %d "$c" >/dev/null 2>&1 || true
  docker rm -f "$c" >/dev/null 2>&1 || true
done
`, containersScript(p.a.ID, p.d.PRNumber), ssh.Quote(keep), p.a.Settings.StopGracePeriod), false)
}

func (p *pipe) runningContainer(ctx context.Context, service string) (string, error) {
	filter := fmt.Sprintf("--filter label=nodra.resource=%s --filter label=nodra.pr=%d", p.a.ID, p.d.PRNumber)
	if service != "" {
		filter += " --filter label=nodra.service=" + ssh.Quote(service)
	}
	out, err := p.out(ctx, "docker ps "+filter+" --filter status=running --format '{{.Names}}' | head -1\n")
	return strings.TrimSpace(out), err
}

func (p *pipe) execIn(ctx context.Context, what, container, command string) error {
	cmd := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(command)
	return p.sh(ctx, fmt.Sprintf("$ %s dans %s : %s", what, container, cmd), "docker exec "+ssh.Quote(container)+" sh -c "+ssh.Quote(cmd)+"\n", false)
}

func (p *pipe) startContainer(ctx context.Context) error {
	a := p.a
	storages, err := p.writeStorageFiles(ctx)
	if err != nil {
		return err
	}
	compose, err := AppCompose(AppSpec{App: a, DeploymentID: p.d.ID, ContainerName: p.containerName, Image: p.image, Network: p.loc.Network,
		ProxyType: p.sv.Proxy.Type, Domains: p.domains, Env: p.env.Runtime, Literal: p.env.Literal, Storages: storages, PR: p.d.PRNumber,
		CaddyCerts: p.certs}, p.appDir+"/files")
	if err != nil {
		return err
	}
	composeFile := p.workdir + "/docker-compose.yml"
	if err := p.sh(ctx, "", writeFile(composeFile, compose, "600"), true); err != nil {
		return err
	}
	p.debug("docker-compose généré :\n%s", Redact(compose, p.log.secrets))
	if a.PreDeploymentCommand != "" {
		if c, _ := p.runningContainer(ctx, ""); c != "" {
			if err := p.execIn(ctx, "commande de pré-déploiement", c, a.PreDeploymentCommand); err != nil {
				return err
			}
		} else {
			p.info("Commande de pré-déploiement ignorée : aucun conteneur en cours d'exécution.")
		}
	}
	if !p.rolling {
		p.info("Mise à jour sans bascule (ports publiés, nom de conteneur fixe ou prévisualisation) : arrêt de l'ancien conteneur.")
		if err := p.removeOld(ctx, ""); err != nil {
			return err
		}
	}
	project := "d" + strings.ReplaceAll(p.d.ID, "-", "")
	if err := p.sh(ctx, "$ docker compose up -d ("+p.containerName+")",
		fmt.Sprintf("docker compose -p %s -f %s up -d --remove-orphans\n", project, ssh.Quote(composeFile)), false); err != nil {
		return err
	}
	if err := p.waitHealthy(ctx, p.containerName); err != nil {
		_ = p.sh(context.WithoutCancel(ctx), "$ docker logs --tail 100 "+p.containerName,
			"docker logs --tail 100 "+ssh.Quote(p.containerName)+" 2>&1 || true\n", false)
		_, _ = p.out(context.WithoutCancel(ctx), "docker rm -f "+ssh.Quote(p.containerName)+" >/dev/null 2>&1 || true\n")
		return err
	}
	if p.rolling {
		if err := p.removeOld(ctx, p.containerName); err != nil {
			return err
		}
	}
	if a.PostDeploymentCommand != "" {
		if err := p.execIn(ctx, "commande de post-déploiement", p.containerName, a.PostDeploymentCommand); err != nil {
			return err
		}
	}
	return p.pruneImages(ctx)
}

// waitHealthy attend que le conteneur soit sain (ou stable s'il n'a pas de vérification de santé).
func (p *pipe) waitHealthy(ctx context.Context, name string) error {
	hc := p.a.HealthCheck
	limit := 60 * time.Second
	if hc.Enabled {
		limit = time.Duration(hc.StartPeriod+hc.Interval*hc.Retries+hc.Timeout)*time.Second + 30*time.Second
	}
	p.info("Attente du démarrage de %s (jusqu'à %s)…", name, limit)
	deadline := time.Now().Add(limit)
	stable := 0
	for time.Now().Before(deadline) {
		out, err := p.out(ctx, "docker inspect -f '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' "+ssh.Quote(name)+"\n")
		if err != nil {
			if ctx.Err() != nil {
				return ErrCancelled
			}
			return fmt.Errorf("conteneur %s introuvable : %w", name, err)
		}
		state, health, _ := strings.Cut(strings.TrimSpace(out), " ")
		switch {
		case state == "exited" || state == "dead" || health == "unhealthy":
			return fmt.Errorf("le conteneur s'est arrêté ou est en mauvaise santé (%s %s)", state, health)
		case state == "running" && health == "healthy":
			p.info("Conteneur sain.")
			return nil
		case state == "running" && health == "":
			if stable++; stable >= 3 {
				p.info("Conteneur démarré.")
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ErrCancelled
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("le conteneur n'est pas devenu sain à temps")
}

// pruneImages garde les N images les plus récentes de l'application (pour les retours arrière).
func (p *pipe) pruneImages(ctx context.Context) error {
	if p.d.PRNumber > 0 {
		return nil
	}
	keep := p.a.Settings.ImagesToKeep
	return p.sh(ctx, "", fmt.Sprintf(`docker images %s --format '{{.Tag}}' | grep -v '^pr' | grep -vx %s | tail -n +%d | while read t; do
  docker rmi %s:"$t" >/dev/null 2>&1 || true
done
`, ssh.Quote("nodra/"+p.a.ID), ssh.Quote(strings.TrimPrefix(p.image, "nodra/"+p.a.ID+":")), keep+1, ssh.Quote("nodra/"+p.a.ID)), true)
}

// --- Docker Compose ---

func dotenv(env map[string]string, literal map[string]bool) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := env[k]
		if literal[k] && !strings.Contains(v, "'") && !strings.Contains(v, "\n") {
			fmt.Fprintf(&b, "%s='%s'\n", k, v) // guillemets simples : aucune interpolation
			continue
		}
		v = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v)
		if literal[k] {
			v = strings.ReplaceAll(v, "$", "$$")
		}
		fmt.Fprintf(&b, "%s=\"%s\"\n", k, v)
	}
	return b.String()
}

func (p *pipe) deployCompose(ctx context.Context) error {
	a := p.a
	raw := a.DockerComposeRaw
	projectDir := p.appDir + "/compose"
	if a.GitRepository != "" {
		url, pre, err := p.repoAccess(ctx)
		if err != nil {
			return err
		}
		sha, err := p.resolveCommit(ctx, url, pre)
		if err != nil {
			return err
		}
		if err := p.clone(ctx, url, pre, sha); err != nil {
			return err
		}
		projectDir = path.Join(p.src, a.BaseDirectory)
		if strings.TrimSpace(raw) == "" {
			out, err := p.out(ctx, "cat "+ssh.Quote(path.Join(projectDir, a.DockerComposeLocation))+"\n")
			if err != nil {
				return fmt.Errorf("fichier %s introuvable dans le dépôt", a.DockerComposeLocation)
			}
			raw = out
		}
	}
	// Preview : domaines propres à la PR et fichiers séparés de ceux de la production.
	prDir := p.appDir
	route := a
	if p.d.PRNumber > 0 {
		prDir = fmt.Sprintf("%s/pr-%d", p.appDir, p.d.PRNumber)
		cp := *a
		cp.DockerComposeDomains = map[string][]string{}
		if pv, err := p.r.Store.PreviewByNumber(ctx, a.ID, p.d.PRNumber); err == nil {
			cp.DockerComposeDomains = pv.ComposeDomains
		}
		route = &cp
	}
	transformed, err := TransformCompose(raw, route, p.d.ID, p.loc.Network, p.sv.Proxy.Type, prDir+"/data", p.certs, p.d.PRNumber)
	if err != nil {
		return err
	}
	composeFile := prDir + "/docker-compose.yml"
	envFile := prDir + "/.env"
	if err := p.sh(ctx, "", fmt.Sprintf("mkdir -p %s %s\n", ssh.Quote(projectDir), ssh.Quote(prDir+"/data"))+
		writeFile(composeFile, transformed, "600")+writeFile(envFile, dotenv(p.env.Runtime, p.env.Literal), "600"), true); err != nil {
		return err
	}
	p.debug("docker-compose transformé :\n%s", Redact(transformed, p.log.secrets))
	if a.PreDeploymentCommand != "" {
		if c, _ := p.runningContainer(ctx, a.PreDeploymentContainer); c != "" {
			if err := p.execIn(ctx, "commande de pré-déploiement", c, a.PreDeploymentCommand); err != nil {
				return err
			}
		}
	}
	project := a.ID
	if p.d.PRNumber > 0 {
		project = fmt.Sprintf("%s-pr-%d", a.ID, p.d.PRNumber)
	}
	base := fmt.Sprintf("docker compose -p %s -f %s --env-file %s --project-directory %s", ssh.Quote(project), ssh.Quote(composeFile),
		ssh.Quote(envFile), ssh.Quote(projectDir))
	noCache := ""
	if p.d.ForceRebuild || a.Settings.DisableBuildCache {
		noCache = " --no-cache"
	}
	if err := p.sh(ctx, "$ docker compose build", base+" build"+noCache+"\n", false); err != nil {
		return err
	}
	if err := p.sh(ctx, "$ docker compose up -d", base+" up -d --remove-orphans\n", false); err != nil {
		return err
	}
	if err := p.waitCompose(ctx, base); err != nil {
		return err
	}
	if a.PostDeploymentCommand != "" {
		c, _ := p.runningContainer(ctx, a.PostDeploymentContainer)
		if c == "" {
			return errors.New("commande de post-déploiement : conteneur introuvable")
		}
		if err := p.execIn(ctx, "commande de post-déploiement", c, a.PostDeploymentCommand); err != nil {
			return err
		}
	}
	p.image = "compose:" + shortSHA(p.d.CommitSHA)
	return nil
}

// waitCompose attend que tous les services soient démarrés (un service terminé avec le code 0 est accepté).
func (p *pipe) waitCompose(ctx context.Context, base string) error {
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := p.out(ctx, base+" ps -a --format '{{.Service}} {{.State}} {{.Health}} {{.ExitCode}}'\n")
		if err != nil {
			return err
		}
		pending := false
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			state, health, code := f[1], "", "0"
			if len(f) == 4 {
				health, code = f[2], f[3]
			} else if len(f) == 3 {
				code = f[2]
			}
			switch {
			case state == "exited" && code != "0", state == "dead", health == "unhealthy":
				return fmt.Errorf("le service %s s'est arrêté ou est en mauvaise santé", f[0])
			case state != "running" && state != "exited", health == "starting":
				pending = true
			}
		}
		if !pending {
			p.info("Tous les services sont démarrés.")
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrCancelled
		case <-time.After(3 * time.Second):
		}
	}
	return errors.New("les services ne sont pas démarrés à temps")
}

// --- Actions hors déploiement ---

func (r *Runner) target(ctx context.Context, resourceID string) (ssh.Target, *store.ResourceLocation, error) {
	loc, err := r.Store.ResourceLocation(ctx, resourceID)
	if err != nil {
		return ssh.Target{}, nil, err
	}
	sv, err := r.Store.ServerByIDAny(ctx, loc.ServerID)
	if err != nil {
		return ssh.Target{}, nil, err
	}
	t, err := r.Servers.Target(ctx, sv)
	return t, loc, err
}

func (r *Runner) run(ctx context.Context, t ssh.Target, script string) (string, error) {
	var out, errb bytes.Buffer
	code, err := r.SSH.RunInput(ctx, t, "sh -s", strings.NewReader(script), &out, &errb)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return out.String(), fmt.Errorf("code %d : %s", code, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// Stop arrête et supprime les conteneurs de l'application (les images et volumes sont conservés).
func (r *Runner) Stop(ctx context.Context, a *store.Application, pr int) error {
	t, _, err := r.target(ctx, a.ID)
	if err != nil {
		return err
	}
	script := fmt.Sprintf("for c in $(%s); do docker stop -t %d \"$c\" >/dev/null 2>&1 || true; docker rm -f \"$c\" >/dev/null 2>&1 || true; done\n",
		containersScript(a.ID, pr), a.Settings.StopGracePeriod)
	if a.BuildPack == "dockercompose" {
		project := a.ID
		if pr > 0 {
			project = fmt.Sprintf("%s-pr-%d", a.ID, pr)
		}
		script = "docker compose -p " + ssh.Quote(project) + " down --remove-orphans >/dev/null 2>&1 || true\n" + script
		if pr > 0 { // fichiers de la preview (recréés au prochain déploiement)
			script += "rm -rf " + ssh.Quote(fmt.Sprintf("%s/%s/pr-%d", r.appsDir(), a.ID, pr)) + "\n"
		}
	}
	if _, err := r.run(ctx, t, script); err != nil {
		return err
	}
	if pr == 0 {
		return r.Store.SetApplicationStatus(ctx, a.ID, "stopped")
	}
	return nil
}

// Teardown supprime conteneurs, images et fichiers de l'application sur le serveur (volumes si demandé).
// Un serveur injoignable n'empêche pas la suppression (les restes sont signalés dans les logs).
func (r *Runner) Teardown(ctx context.Context, a *store.Application, deleteVolumes bool) error {
	t, _, err := r.target(ctx, a.ID)
	if err != nil {
		r.Log.Warn("suppression : serveur inaccessible", "application", a.ID, "err", err)
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "for c in $(docker ps -aq --filter label=nodra.resource=%s); do docker rm -f \"$c\" >/dev/null 2>&1 || true; done\n", a.ID)
	fmt.Fprintf(&b, "for p in $(docker compose ls -aq 2>/dev/null | grep '^%s' || true); do docker compose -p \"$p\" down --remove-orphans %s >/dev/null 2>&1 || true; done\n",
		a.ID, map[bool]string{true: "-v", false: ""}[deleteVolumes])
	fmt.Fprintf(&b, "docker images -q %s | sort -u | xargs -r docker rmi -f >/dev/null 2>&1 || true\n", ssh.Quote("nodra/"+a.ID))
	if deleteVolumes {
		storages, err := r.Store.ListStorages(ctx, a.ID)
		if err == nil {
			for _, s := range storages {
				if s.Kind == "volume" {
					fmt.Fprintf(&b, "docker volume rm %s >/dev/null 2>&1 || true\n", ssh.Quote(s.Name))
				}
			}
		}
	}
	fmt.Fprintf(&b, "rm -rf %s\n", ssh.Quote(r.appsDir()+"/"+a.ID))
	if _, err := r.run(ctx, t, b.String()); err != nil {
		r.Log.Warn("suppression : nettoyage incomplet", "application", a.ID, "err", err)
	}
	return nil
}

// ContainerLogs renvoie les dernières lignes de logs des conteneurs de l'application.
func (r *Runner) ContainerLogs(ctx context.Context, a *store.Application, pr, lines int) (string, error) {
	t, _, err := r.target(ctx, a.ID)
	if err != nil {
		return "", err
	}
	return r.run(ctx, t, fmt.Sprintf(`for c in $(docker ps -a --filter label=nodra.resource=%s --filter label=nodra.pr=%d --format '{{.Names}}'); do
  echo "==> $c"
  docker logs --tail %d --timestamps "$c" 2>&1
done
`, a.ID, pr, lines))
}

type Image struct {
	Tag     string `json:"tag"`
	Image   string `json:"image"`
	Created string `json:"created"`
	Size    string `json:"size"`
	Current bool   `json:"current"`
}

// Images liste les images de l'application disponibles pour un retour arrière.
func (r *Runner) Images(ctx context.Context, a *store.Application) ([]Image, error) {
	t, _, err := r.target(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	out, err := r.run(ctx, t, "docker images "+ssh.Quote("nodra/"+a.ID)+" --format '{{.Tag}}|{{.CreatedAt}}|{{.Size}}'\n")
	if err != nil {
		return nil, err
	}
	current, _ := r.Store.LastDeployedImage(ctx, a.ID, 0)
	images := []Image{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "|")
		if len(f) != 3 || strings.HasPrefix(f[0], "pr") || f[0] == "<none>" {
			continue
		}
		img := "nodra/" + a.ID + ":" + f[0]
		images = append(images, Image{Tag: f[0], Image: img, Created: f[1], Size: f[2], Current: img == current})
	}
	return images, nil
}

var _ io.Writer = logWriter{}

// registries écrit les identifiants des registres privés de l'équipe et les active pour la suite.
func (p *pipe) registries(ctx context.Context) error {
	setup, env, err := p.r.dockerConfig(ctx, p.a.TeamID)
	if err != nil {
		return err
	}
	if _, err := p.out(ctx, setup); err != nil {
		return fmt.Errorf("identifiants des registres : %w", err)
	}
	if env != "" {
		p.debug("Registres privés de l'équipe activés.")
	}
	p.dockerEnv = env
	return nil
}
