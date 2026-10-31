import { useMutation, useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { Section } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { ApiError, get, post, teamPath } from "@/lib/api"
import type { GitSource, PrivateKey, Repo } from "@/lib/types"
import type { Kind } from "./options"

type Ctx = { team: string; environment: string; onCreated: (id: string) => void }

const BUILD_PACKS = [
  { id: "nixpacks", label: "Nixpacks (détection automatique)" },
  { id: "static", label: "Site statique" },
  { id: "dockerfile", label: "Dockerfile du dépôt" },
  { id: "dockercompose", label: "Docker Compose du dépôt" },
]

/** Création d'une application sur le serveur de l'équipe ; les champs vides prennent les valeurs par défaut de l'API. */
function useCreateApp({ team, environment, onCreated }: Ctx) {
  return useMutation({
    mutationFn: (body: Record<string, unknown>) => post<{ id: string }>(teamPath(team, "/applications"), { environment_id: environment, ...body }),
    onSuccess: (a) => onCreated(a.id),
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Création impossible"),
  })
}

const repoName = (url: string) =>
  url
    .replace(/\.git$/, "")
    .split(/[/:]/)
    .filter(Boolean)
    .pop() ?? ""

/** Réglages de build communs aux dépôts Git (comme « Build configuration » de Coolify). */
function BuildFields({ v, set }: { v: Build; set: (b: Build) => void }) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <Field label="Branche" hint="Modifiable après la création.">
        <Input className="font-mono" value={v.branch} onChange={(e) => set({ ...v, branch: e.target.value })} />
      </Field>
      <Field label="Build pack">
        <Select value={v.buildPack} onValueChange={(b) => set({ ...v, buildPack: b })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {BUILD_PACKS.map((b) => (
              <SelectItem key={b.id} value={b.id}>
                {b.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <Field label="Dossier de base" hint="Dossier du dépôt utilisé comme racine du build.">
        <Input className="font-mono" value={v.baseDir} onChange={(e) => set({ ...v, baseDir: e.target.value })} />
      </Field>
      {v.buildPack === "static" ? (
        <Field label="Dossier publié" hint="Dossier contenant les fichiers générés (ex. /dist).">
          <Input className="font-mono" value={v.publishDir} onChange={(e) => set({ ...v, publishDir: e.target.value })} />
        </Field>
      ) : (
        v.buildPack !== "dockercompose" && (
          <Field label="Port" hint="Port écouté par l'application.">
            <Input className="font-mono" value={v.port} onChange={(e) => set({ ...v, port: e.target.value })} />
          </Field>
        )
      )}
    </div>
  )
}

type Build = { branch: string; buildPack: string; baseDir: string; port: string; publishDir: string }
const BUILD: Build = { branch: "main", buildPack: "nixpacks", baseDir: "/", port: "3000", publishDir: "/dist" }

const buildBody = (b: Build) => ({
  git_branch: b.branch.trim() || "main",
  build_pack: b.buildPack,
  base_directory: b.baseDir.trim() || "/",
  ...(b.buildPack === "static" ? { publish_directory: b.publishDir.trim() } : b.buildPack !== "dockercompose" ? { ports_exposes: b.port.trim() } : {}),
})

function Submit({ pending, disabled }: { pending: boolean; disabled?: boolean }) {
  return (
    <div className="flex justify-end">
      <Button type="submit" loading={pending} disabled={disabled}>
        Continuer
      </Button>
    </div>
  )
}

export function PublicRepoForm(ctx: Ctx) {
  const create = useCreateApp(ctx)
  const [url, setUrl] = useState("")
  const [b, setB] = useState(BUILD)
  return (
    <Section title="Dépôt Git public" description="Collez l'URL d'un dépôt public (GitHub, GitLab, Gitea, Bitbucket…).">
      <form
        className="space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate({ name: repoName(url), git_repository: url.trim(), ...buildBody(b) })
        }}
      >
        <Field label="URL du dépôt">
          <Input required autoFocus className="font-mono" placeholder="https://github.com/owner/repository" value={url} onChange={(e) => setUrl(e.target.value)} />
        </Field>
        <BuildFields v={b} set={setB} />
        <Submit pending={create.isPending} disabled={!url.trim()} />
      </form>
    </Section>
  )
}

export function DeployKeyForm(ctx: Ctx) {
  const create = useCreateApp(ctx)
  const keys = useQuery({ queryKey: ["team", ctx.team, "private-keys"], queryFn: () => get<PrivateKey[]>(teamPath(ctx.team, "/private-keys")) })
  const [key, setKey] = useState("")
  const [url, setUrl] = useState("")
  const [b, setB] = useState(BUILD)
  return (
    <Section title="Dépôt privé (clé de déploiement)" description="La clé publique choisie doit être ajoutée comme clé de déploiement du dépôt.">
      <form
        className="space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate({ name: repoName(url), git_repository: url.trim(), private_key_id: key, ...buildBody(b) })
        }}
      >
        <Field label="Clé SSH" hint={keys.data?.length === 0 ? "Aucune clé : créez-en une dans Clés et jetons." : undefined}>
          <Select value={key} onValueChange={setKey}>
            <SelectTrigger>
              <SelectValue placeholder="Choisir une clé…" />
            </SelectTrigger>
            <SelectContent>
              {keys.data?.map((k) => (
                <SelectItem key={k.id} value={k.id}>
                  {k.name} · {k.fingerprint.slice(0, 20)}…
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label="URL du dépôt">
          <Input required className="font-mono" placeholder="git@github.com:owner/repository.git" value={url} onChange={(e) => setUrl(e.target.value)} />
        </Field>
        <BuildFields v={b} set={setB} />
        <Submit pending={create.isPending} disabled={!url.trim() || !key} />
      </form>
    </Section>
  )
}

export function SourceRepoForm({ type, ...ctx }: Ctx & { type: "github" | "gitlab" | "gitea" }) {
  const create = useCreateApp(ctx)
  const sources = useQuery({ queryKey: ["team", ctx.team, "sources"], queryFn: () => get<GitSource[]>(teamPath(ctx.team, "/sources")) })
  const list = sources.data?.filter((s) => s.type === type) ?? []
  const [source, setSource] = useState("")
  const [repo, setRepo] = useState("")
  const [b, setB] = useState(BUILD)
  const selected = source || (list.length === 1 ? list[0].id : "")
  const repos = useQuery({
    queryKey: ["team", ctx.team, "source", selected, "repos"],
    queryFn: () => get<Repo[]>(teamPath(ctx.team, `/sources/${selected}/repositories`)),
    enabled: !!selected,
  })
  const branches = useQuery({
    queryKey: ["team", ctx.team, "source", selected, "branches", repo],
    queryFn: () => get<string[]>(teamPath(ctx.team, `/sources/${selected}/branches?repo=${encodeURIComponent(repo)}`)),
    enabled: !!selected && !!repo,
  })
  const label = type === "github" ? "GitHub" : type === "gitlab" ? "GitLab" : "Gitea"
  if (sources.isSuccess && !list.length)
    return (
      <Section title={`Dépôt Git (${label})`}>
        <p className="text-sm text-muted-foreground">Aucune source {label} connectée. Ajoutez-en une dans Intégrations, puis revenez ici.</p>
      </Section>
    )
  return (
    <Section title={`Dépôt Git (${label})`} description="Choisissez la source, le dépôt puis la branche.">
      <form
        className="space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate({ name: repo.split("/").pop(), git_repository: repo, source_id: selected, ...buildBody(b) })
        }}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Source">
            <Select value={selected} onValueChange={(v) => (setSource(v), setRepo(""))}>
              <SelectTrigger>
                <SelectValue placeholder="Choisir…" />
              </SelectTrigger>
              <SelectContent>
                {list.map((s) => (
                  <SelectItem key={s.id} value={s.id}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field label="Dépôt" error={repos.isError ? (repos.error instanceof ApiError ? repos.error.message : "Liste indisponible") : undefined}>
            <Select
              value={repo}
              onValueChange={(v) => {
                setRepo(v)
                const def = repos.data?.find((r) => r.full_name === v)?.default_branch
                if (def) setB((x) => ({ ...x, branch: def }))
              }}
              disabled={!repos.data}
            >
              <SelectTrigger>
                <SelectValue placeholder={repos.isFetching ? "Chargement…" : "Choisir…"} />
              </SelectTrigger>
              <SelectContent>
                {repos.data?.map((r) => (
                  <SelectItem key={r.full_name} value={r.full_name}>
                    {r.full_name}
                    {r.private ? " (privé)" : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        </div>
        {repo && (
          <>
            {branches.data && branches.data.length > 0 && (
              <Field label="Branches disponibles">
                <Select value={b.branch} onValueChange={(v) => setB({ ...b, branch: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {branches.data.map((br) => (
                      <SelectItem key={br} value={br}>
                        {br}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            )}
            <BuildFields v={b} set={setB} />
          </>
        )}
        <Submit pending={create.isPending} disabled={!repo} />
      </form>
    </Section>
  )
}

export function DockerfileForm(ctx: Ctx) {
  const create = useCreateApp(ctx)
  const [f, setF] = useState({ dockerfile: "FROM nginx:alpine\nCOPY . /usr/share/nginx/html\n", port: "80" })
  return (
    <Section title="Dockerfile" description="Le Dockerfile est construit sur le serveur, sans dépôt Git.">
      <form
        className="space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate({ name: "dockerfile", build_pack: "dockerfile", dockerfile: f.dockerfile, ports_exposes: f.port.trim() })
        }}
      >
        <Field label="Dockerfile">
          <Textarea rows={12} className="font-mono text-xs" value={f.dockerfile} onChange={(e) => setF({ ...f, dockerfile: e.target.value })} />
        </Field>
        <Field label="Port" hint="Port écouté par le conteneur." className="max-w-48">
          <Input className="font-mono" value={f.port} onChange={(e) => setF({ ...f, port: e.target.value })} />
        </Field>
        <Submit pending={create.isPending} disabled={!f.dockerfile.trim()} />
      </form>
    </Section>
  )
}

export function ComposeForm(ctx: Ctx) {
  const create = useCreateApp(ctx)
  const [compose, setCompose] = useState("services:\n  web:\n    image: nginx:alpine\n")
  return (
    <Section title="Docker Compose" description="Collez un fichier Docker Compose ; les domaines se règlent par service après la création.">
      <form
        className="space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate({ name: "compose", build_pack: "dockercompose", docker_compose_raw: compose })
        }}
      >
        <Field label="Fichier Docker Compose">
          <Textarea rows={14} className="font-mono text-xs" value={compose} onChange={(e) => setCompose(e.target.value)} />
        </Field>
        <Submit pending={create.isPending} disabled={!compose.trim()} />
      </form>
    </Section>
  )
}

export function ImageForm(ctx: Ctx) {
  const create = useCreateApp(ctx)
  const [f, setF] = useState({ image: "", tag: "", port: "80" })
  return (
    <Section title="Image Docker" description="Image publique ou d'un registre privé déclaré dans Intégrations.">
      <form
        className="space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          const [image, inlineTag] = f.image.trim().split(/:(?=[^/]+$)/)
          create.mutate({ name: repoName(image), build_pack: "dockerimage", docker_image: image, docker_image_tag: f.tag.trim() || inlineTag || "latest", ports_exposes: f.port.trim() })
        }}
      >
        <Field label="Nom de l'image" hint="Référence complète (nginx, ghcr.io/user/app:v1.2.3) ou nom seul avec le tag ci-dessous.">
          <Input required autoFocus className="font-mono" placeholder="nginx, ghcr.io/user/app:v1.2.3" value={f.image} onChange={(e) => setF({ ...f, image: e.target.value })} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Tag" hint="Par défaut : celui de la référence, sinon latest.">
            <Input className="font-mono" placeholder="latest" value={f.tag} onChange={(e) => setF({ ...f, tag: e.target.value })} />
          </Field>
          <Field label="Port" hint="Port écouté par le conteneur.">
            <Input className="font-mono" value={f.port} onChange={(e) => setF({ ...f, port: e.target.value })} />
          </Field>
        </div>
        <Submit pending={create.isPending} disabled={!f.image.trim()} />
      </form>
    </Section>
  )
}

export function AppForm({ kind, ...ctx }: Ctx & { kind: Kind }) {
  switch (kind) {
    case "public":
      return <PublicRepoForm {...ctx} />
    case "deploy-key":
      return <DeployKeyForm {...ctx} />
    case "github":
    case "gitlab":
    case "gitea":
      return <SourceRepoForm type={kind} {...ctx} />
    case "dockerfile":
      return <DockerfileForm {...ctx} />
    case "compose":
      return <ComposeForm {...ctx} />
    case "image":
      return <ImageForm {...ctx} />
    default:
      return null
  }
}
