import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { ArrowRightIcon, ChevronDownIcon, ExternalLinkIcon, GitBranchIcon, PlusIcon, RefreshCwIcon, SettingsIcon, Trash2Icon, TriangleAlertIcon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { api, get, post } from "@/lib/api"
import { cn } from "@/lib/utils"

export type Source = {
  id: string
  type: "github" | "gitlab" | "gitea"
  name: string
  html_url: string
  api_url: string
  organization: string
  is_public: boolean
  app_id: number | null
  app_slug: string
  client_id: string
  installation_id: number | null
  configured: boolean
  installed: boolean
  has_private_key: boolean
  has_webhook_secret: boolean
  has_access_token: boolean
  created_at: string
}

export const sourcesQuery = (team: string) => ({ queryKey: ["team", team, "sources"], queryFn: () => get<Source[]>(`/teams/${team}/sources`) })

/** État d'une GitHub App, comme les pastilles du fork. */
export function sourceState(s: Source): { label: string; dot: string } {
  if (s.type !== "github") return s.has_access_token ? { label: "Connectée", dot: "bg-success" } : { label: "Sans jeton", dot: "bg-warning" }
  if (s.installed) return { label: "Installée", dot: "bg-success" }
  if (s.configured) return { label: "À installer", dot: "bg-warning" }
  return { label: "Configuration incomplète", dot: "bg-primary" }
}

// Nom proposé par défaut (adjectif-animal-suffixe, comme Coolify).
const ADJ = ["brave", "calme", "agile", "vif", "sage", "rapide", "discret", "joyeux"]
const ANIMALS = ["loutre", "faucon", "renard", "lynx", "castor", "colibri", "blaireau", "heron"]
const randomName = () => `${ADJ[Math.floor(Math.random() * ADJ.length)]}-${ANIMALS[Math.floor(Math.random() * ANIMALS.length)]}-${Math.random().toString(36).slice(2, 10)}`

// Page de retour (nouvelle ressource) gardée le temps de passer par GitHub.
const returnKey = (id: string) => `nodra-github-app-return-${id}`
const readReturn = (id: string) => {
  try {
    return sessionStorage.getItem(returnKey(id)) ?? ""
  } catch {
    return ""
  }
}

/** Dialogue « Nouvelle GitHub App » (source/github/create du fork) : enregistre l'App en brouillon puis ouvre sa page. */
export function NewGitHubAppDialog({ team, open, onOpenChange, returnTo }: { team: string; open: boolean; onOpenChange: (o: boolean) => void; returnTo?: string }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [d, setD] = useState(() => ({ name: randomName(), organization: "", html_url: "https://github.com", api_url: "https://api.github.com" }))
  const [previews, setPreviews] = useState(true)
  const [enterprise, setEnterprise] = useState(false)
  // Comme le fork : l'adresse de l'API suit l'adresse HTML tant qu'elle n'a pas été modifiée à la main.
  const apiFor = (html: string) => (html.replace(/\/$/, "") === "https://github.com" ? "https://api.github.com" : `${html.replace(/\/$/, "")}/api/v3`)
  const create = useMutation({
    mutationFn: () => post<Source>(`/teams/${team}/sources`, { type: "github", ...d, organization: d.organization || undefined }),
    onSuccess: (src) => {
      try {
        if (returnTo) sessionStorage.setItem(returnKey(src.id), returnTo)
      } catch {
        // stockage indisponible : pas de retour automatique
      }
      void qc.invalidateQueries({ queryKey: ["team", team, "sources"] })
      onOpenChange(false)
      void navigate({ to: "/t/$team/integrations/sources/$source", params: { team, source: src.id }, search: { previews: previews ? "1" : "0" } })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Nouvelle GitHub App</DialogTitle>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate()
          }}
        >
          <p className="text-xs text-muted-foreground">Connectez une GitHub App pour les dépôts privés, les webhooks et les déploiements de commits et de pull requests.</p>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Nom *">
              <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} required />
            </Field>
            <Field label="Organisation" hint="Si vide, votre compte GitHub est utilisé.">
              <Input value={d.organization} onChange={(e) => setD({ ...d, organization: e.target.value })} placeholder="Compte personnel si vide" />
            </Field>
          </div>
          <div>
            <p className="text-xs font-medium text-muted-foreground">Utiliser cette App pour</p>
            <p className="mt-1 text-xs text-muted-foreground">Le déploiement depuis des dépôts privés est toujours inclus. Les choix ci-dessous fixent les permissions et les événements de webhook à la page suivante.</p>
            <label className="mt-3 flex items-center gap-3 text-sm" title="Déploie les pull requests et publie l'état du déploiement dessus.">
              <Checkbox checked={previews} onCheckedChange={(c) => setPreviews(c === true)} /> Previews des pull requests
            </label>
          </div>
          <div className="rounded-xl border border-border">
            <button type="button" onClick={() => setEnterprise(!enterprise)} className="flex w-full items-center justify-between px-3 py-2.5 text-left text-xs font-medium">
              GitHub auto-hébergé / Enterprise
              <ChevronDownIcon className={cn("size-3.5 transition-transform", enterprise && "rotate-180")} />
            </button>
            {enterprise && (
              <div className="grid gap-4 border-t border-border p-3 sm:grid-cols-2">
                <Field label="URL HTML *" hint="Pour GitHub Enterprise, l'adresse de votre instance (ex. https://github.exemple.fr).">
                  <Input
                    value={d.html_url}
                    onChange={(e) => setD({ ...d, html_url: e.target.value, api_url: d.api_url === apiFor(d.html_url) ? apiFor(e.target.value) : d.api_url })}
                    className="font-mono"
                    required
                  />
                </Field>
                <Field label="URL de l'API *" hint="En général https://api.github.com ou l'API de votre Enterprise.">
                  <Input value={d.api_url} onChange={(e) => setD({ ...d, api_url: e.target.value })} className="font-mono" required />
                </Field>
              </div>
            )}
          </div>
          <Button type="submit" className="w-full" disabled={!d.name.trim()} loading={create.isPending}>
            Continuer
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Enregistrement d'une App brouillon (source/github/change du fork) : installation automatique ou manuelle. */
export function RegisterGitHubApp({ team, source, previewsDefault }: { team: string; source: Source; previewsDefault: boolean }) {
  const endpoints = useQuery({ queryKey: ["team", team, "github-endpoints"], queryFn: () => get<{ label: string; url: string }[]>(`/teams/${team}/sources/github/endpoints`) }).data ?? []
  const [mode, setMode] = useState<"instance" | "custom">("instance")
  const [endpoint, setEndpoint] = useState("")
  const [custom, setCustom] = useState("")
  const [previews, setPreviews] = useState(previewsDefault ? "write" : "none")
  const chosen = mode === "custom" ? custom : endpoint || endpoints[0]?.url || ""
  const register = useMutation({
    mutationFn: () => post<{ form_action: string; manifest: string }>(`/teams/${team}/sources/${source.id}/github/manifest`, { endpoint: chosen, previews: previews === "write" }),
    onSuccess: (r) => submitManifest(r),
    onError: (e) => toast.error(e.message),
  })
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <SettingsCard title="Installation automatique" actions={<Pill label="Recommandée" dot="bg-success" />}>
        <div className="space-y-4">
          <Field label="Point de webhook" hint="Adresse à laquelle GitHub envoie les webhooks et renvoie après l'enregistrement.">
            <Select value={mode} onValueChange={(v) => setMode(v as typeof mode)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="instance">Utiliser une adresse de l'instance</SelectItem>
                <SelectItem value="custom">Utiliser une adresse personnalisée</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          {mode === "instance" ? (
            <Field label="Adresse de l'instance">
              <Select value={endpoint || endpoints[0]?.url || ""} onValueChange={setEndpoint}>
                <SelectTrigger>
                  <SelectValue placeholder="Chargement…" />
                </SelectTrigger>
                <SelectContent>
                  {endpoints.map((e) => (
                    <SelectItem key={e.url} value={e.url}>
                      Utiliser {e.url}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          ) : (
            <Field label="Adresse personnalisée">
              <Input value={custom} onChange={(e) => setCustom(e.target.value)} placeholder="https://nodra.exemple.fr" className="font-mono" />
            </Field>
          )}
          <div className="rounded-xl border border-border px-4 py-3 text-sm">
            <p className="font-medium">Permissions obligatoires</p>
            <p className="mt-1 text-muted-foreground">Contenu : lecture · Métadonnées : lecture · E-mail : lecture · Administration : lecture</p>
          </div>
          <Field label="Accès pour les previews de déploiement" hint="Droits sur les pull requests pour déployer les previews et y publier leur état.">
            <Select value={previews} onValueChange={setPreviews}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="write">Lire et modifier les pull requests</SelectItem>
                <SelectItem value="none">Pas de previews</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Button className="w-full" disabled={!chosen} loading={register.isPending} onClick={() => register.mutate()}>
            S'enregistrer sur GitHub
          </Button>
        </div>
      </SettingsCard>
      <ManualInstall team={team} source={source} />
    </div>
  )
}

/** Soumet le manifeste à GitHub (formulaire POST), qui crée l'App puis revient sur l'instance. */
function submitManifest(p: { form_action: string; manifest: string }) {
  const form = document.createElement("form")
  form.method = "POST"
  form.action = p.form_action
  const input = document.createElement("input")
  input.type = "hidden"
  input.name = "manifest"
  input.value = p.manifest
  form.append(input)
  document.body.append(form)
  form.submit()
}

function ManualInstall({ team, source }: { team: string; source: Source }) {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [d, setD] = useState({ app_id: "", installation_id: "", client_id: "", client_secret: "", webhook_secret: "", private_key: "" })
  const save = useMutation({
    mutationFn: () =>
      api(`/teams/${team}/sources/${source.id}`, {
        method: "PATCH",
        json: { ...d, app_id: Number(d.app_id), installation_id: d.installation_id ? Number(d.installation_id) : undefined },
      }),
    onSuccess: () => {
      toast.success("GitHub App enregistrée")
      void qc.invalidateQueries({ queryKey: ["team", team, "source", source.id] })
      void qc.invalidateQueries({ queryKey: ["team", team, "sources"] })
    },
    onError: (e) => toast.error(e.message),
  })
  const f = (k: keyof typeof d, label: string) => (
    <Field label={label}>
      <Input value={d[k]} onChange={(e) => setD({ ...d, [k]: e.target.value })} className="font-mono" type={k.includes("secret") || k === "client_id" ? "password" : "text"} />
    </Field>
  )
  return (
    <SettingsCard title="Installation manuelle" actions={<Pill label="Avancé" dot="bg-muted-foreground" />}>
      {!open ? (
        <div className="flex h-full min-h-72 flex-col justify-between gap-6">
          <div className="space-y-6">
            <span className="flex size-10 items-center justify-center rounded-xl border border-border bg-muted text-muted-foreground">
              <SettingsIcon className="size-5" />
            </span>
            <p className="text-sm text-muted-foreground">À utiliser quand l'enregistrement automatique par GitHub n'est pas possible, ou pour garder la main sur toute la configuration de l'App.</p>
          </div>
          <Button variant="outline" className="self-start" onClick={() => setOpen(true)}>
            Continuer manuellement <ArrowRightIcon />
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            {f("app_id", "App ID *")}
            {f("installation_id", "Installation ID")}
            {f("client_id", "Client ID")}
            {f("client_secret", "Client secret")}
            {f("webhook_secret", "Secret du webhook *")}
          </div>
          <Field label="Clé privée de l'App (PEM) *">
            <Textarea value={d.private_key} onChange={(e) => setD({ ...d, private_key: e.target.value })} rows={6} className="font-mono" />
          </Field>
          <Field label="URL du webhook à déclarer dans l'App">
            <CopyInput value={`${location.origin}/webhooks/github`} />
          </Field>
          <Button disabled={!d.app_id || !d.private_key || !d.webhook_secret} loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
        </div>
      )}
    </SettingsCard>
  )
}

const installUrl = (s: Source) => `${s.html_url}${s.html_url === "https://github.com" ? "/apps" : "/github-apps"}/${s.app_slug}/installations/new`

/** Page d'une source : enregistrement, installation, puis réglages (comme source/github/change du fork). */
export function SourceGeneral({ team, id, installed, previews }: { team: string; id: string; installed?: boolean; previews?: string }) {
  const source = useQuery({ queryKey: ["team", team, "source", id], queryFn: () => get<Source>(`/teams/${team}/sources/${id}`) }).data
  if (!source) return null
  const returnTo = readReturn(id)
  if (source.type === "github" && !source.app_id) {
    return (
      <>
        <PageHeader title={source.name} description="Terminez l'enregistrement de cette GitHub App avant de l'utiliser comme source." />
        <RegisterGitHubApp team={team} source={source} previewsDefault={previews !== "0"} />
      </>
    )
  }
  return (
    <>
      <PageHeader title={source.name} description={source.organization ? `GitHub App de ${source.organization}` : "Source GitHub privée"} />
      <div className="space-y-6">
        {installed && returnTo && (
          <SettingsCard title="Installation terminée" actions={<Button asChild><Link to={returnTo}>Continuer la création <ArrowRightIcon /></Link></Button>}>
            <p className="text-sm text-muted-foreground">La GitHub App a accès à vos dépôts : vous pouvez reprendre la création de la ressource.</p>
          </SettingsCard>
        )}
        {source.type === "github" && !source.installed ? (
          <SettingsCard
            title="Terminer l'installation GitHub"
            description="Choisissez les dépôts auxquels cette GitHub App a accès avant de l'utiliser comme source."
            actions={
              <Button asChild>
                <a href={installUrl(source)}>
                  Installer sur des dépôts <ExternalLinkIcon />
                </a>
              </Button>
            }
          >
            <div className="flex items-start gap-3">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-warning/10 text-warning">
                <TriangleAlertIcon className="size-4" />
              </span>
              <p className="text-sm text-muted-foreground">L'accès aux dépôts n'a pas encore été installé. Terminez cette étape avant d'utiliser la source pour une application.</p>
            </div>
          </SettingsCard>
        ) : (
          <SourceSettings key={JSON.stringify(source)} team={team} source={source} />
        )}
      </div>
    </>
  )
}

function SourceSettings({ team, source }: { team: string; source: Source }) {
  const qc = useQueryClient()
  const [d, setD] = useState({ name: source.name, organization: source.organization, html_url: source.html_url, api_url: source.api_url, app_id: String(source.app_id ?? ""), installation_id: String(source.installation_id ?? ""), client_id: source.client_id ?? "", client_secret: "", webhook_secret: "", private_key: "", access_token: "" })
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["team", team, "source", source.id] })
    void qc.invalidateQueries({ queryKey: ["team", team, "sources"] })
  }
  const save = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = { name: d.name, organization: d.organization, html_url: d.html_url, api_url: d.api_url }
      if (source.type === "github") {
        body.app_id = Number(d.app_id)
        body.installation_id = Number(d.installation_id)
        body.client_id = d.client_id
        for (const k of ["client_secret", "webhook_secret", "private_key"] as const) if (d[k]) body[k] = d[k]
      } else if (d.access_token) body.access_token = d.access_token
      return api(`/teams/${team}/sources/${source.id}`, { method: "PATCH", json: body })
    },
    onSuccess: () => (toast.success("Source enregistrée"), refresh()),
    onError: (e) => toast.error(e.message),
  })
  const test = useMutation({
    mutationFn: () => post<{ ok: boolean; repositories?: number; message?: string }>(`/teams/${team}/sources/${source.id}/test`),
    onSuccess: (r) => (r.ok ? toast.success(`Connexion réussie : ${r.repositories} dépôt(s) accessible(s)`) : toast.error(r.message ?? "Connexion impossible")),
    onError: (e) => toast.error(e.message),
  })
  const sync = useMutation({
    mutationFn: () => post(`/teams/${team}/sources/${source.id}/github/sync-name`),
    onSuccess: () => (toast.success("Nom synchronisé depuis GitHub"), refresh()),
    onError: (e) => toast.error(e.message),
  })
  const f = (k: keyof typeof d, label: string, secret?: boolean, hint?: string) => (
    <Field label={label} hint={hint}>
      <Input value={d[k]} onChange={(e) => setD({ ...d, [k]: e.target.value })} type={secret ? "password" : "text"} className="font-mono" />
    </Field>
  )
  const gh = source.type === "github"
  return (
    <SettingsCard
      title="Général"
      description="Connexion et authentification de cette source privée."
      actions={
        <>
          <Button size="sm" variant="outline" loading={test.isPending} onClick={() => test.mutate()}>
            Tester la connexion
          </Button>
          {gh && (
            <>
              <Button size="sm" variant="outline" loading={sync.isPending} onClick={() => sync.mutate()}>
                <RefreshCwIcon /> Synchroniser le nom
              </Button>
              <Button size="sm" variant="outline" asChild>
                <a href={`${source.html_url}${source.organization ? `/organizations/${source.organization}` : ""}/settings/apps/${source.app_slug}`} target="_blank" rel="noreferrer">
                  Renommer <ExternalLinkIcon />
                </a>
              </Button>
              <Button size="sm" variant="outline" asChild>
                <a href={installUrl(source)} target="_blank" rel="noreferrer">
                  Dépôts <ExternalLinkIcon />
                </a>
              </Button>
            </>
          )}
          <Button size="sm" loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
        </>
      }
    >
      <div className="grid gap-4 lg:grid-cols-2">
        {f("name", gh ? "Nom de l'App" : "Nom")}
        {f("organization", "Organisation")}
        {f("html_url", "URL HTML")}
        {f("api_url", "URL de l'API")}
        {gh ? (
          <>
            {f("app_id", "App ID *")}
            {f("installation_id", "Installation ID *")}
            {f("client_id", "Client ID", true)}
            {f("client_secret", "Client secret", true, "Vide = inchangé.")}
            {f("webhook_secret", "Secret du webhook", true, "Vide = inchangé.")}
            <Field label="Clé privée (PEM)" hint="Vide = inchangée." className="lg:col-span-2">
              <Textarea value={d.private_key} onChange={(e) => setD({ ...d, private_key: e.target.value })} rows={4} className="font-mono" placeholder={source.has_private_key ? "•••••••• (enregistrée)" : ""} />
            </Field>
          </>
        ) : (
          f("access_token", "Jeton d'accès", true, "Vide = inchangé.")
        )}
      </div>
    </SettingsCard>
  )
}

/** Onglet « Permissions » : permissions et événements accordés à l'App chez GitHub. */
export function SourcePermissions({ team, id }: { team: string; id: string }) {
  const app = useQuery({ queryKey: ["team", team, "source", id, "github-app"], queryFn: () => get<{ permissions: Record<string, string>; events: string[] }>(`/teams/${team}/sources/${id}/github/app`), retry: false })
  return (
    <>
      <PageHeader title="Permissions" description="Ce que GitHub accorde à l'App. Les modifier se fait dans les réglages de l'App sur GitHub." />
      <div className="space-y-6">
        <SettingsCard title="Permissions" bodyClassName="p-0">
          {app.error && <p className="px-5 py-6 text-sm text-destructive">{app.error.message}</p>}
          {Object.entries(app.data?.permissions ?? {}).map(([k, v]) => (
            <div key={k} className="flex items-center justify-between border-b border-border px-4 py-2.5 text-sm last:border-b-0">
              <span className="font-mono">{k}</span>
              <Pill label={v === "write" ? "Lecture et écriture" : v === "read" ? "Lecture" : v} dot={v === "write" ? "bg-warning" : "bg-success"} />
            </div>
          ))}
        </SettingsCard>
        <SettingsCard title="Événements de webhook">
          <div className="flex flex-wrap gap-2">
            {(app.data?.events ?? []).map((e) => (
              <span key={e} className="rounded-full border border-border bg-muted px-3 py-1 font-mono text-xs">
                {e}
              </span>
            ))}
            {app.data && !app.data.events.length && <p className="text-sm text-muted-foreground">Aucun événement.</p>}
          </div>
        </SettingsCard>
      </div>
    </>
  )
}

/** Onglet « Ressources » : applications qui utilisent la source. */
export function SourceResources({ team, id }: { team: string; id: string }) {
  const apps = useQuery({ queryKey: ["team", team, "applications"], queryFn: () => get<{ id: string; name: string; git_repository: string; git_branch: string; source_id: string | null }[]>(`/teams/${team}/applications`) }).data ?? []
  const list = apps.filter((a) => a.source_id === id)
  return (
    <>
      <PageHeader title="Ressources" description="Applications déployées depuis cette source." />
      <SettingsCard title="Applications" bodyClassName="p-0">
        {list.map((a) => (
          <Link key={a.id} to="/t/$team/applications/$application" params={{ team, application: a.id }} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0 hover:bg-foreground/[0.03]">
            <span className="min-w-0 flex-1 truncate text-sm font-semibold">{a.name}</span>
            <span className="truncate font-mono text-xs text-muted-foreground">
              {a.git_repository} · {a.git_branch}
            </span>
          </Link>
        ))}
        {list.length === 0 && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune application n'utilise cette source.</p>}
      </SettingsCard>
    </>
  )
}

/** Onglet « Zone de danger » : suppression de la source. */
export function SourceDanger({ team, id }: { team: string; id: string }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const source = useQuery({ queryKey: ["team", team, "source", id], queryFn: () => get<Source>(`/teams/${team}/sources/${id}`) }).data
  const [open, setOpen] = useState(false)
  const remove = useMutation({
    mutationFn: () => api(`/teams/${team}/sources/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["team", team, "sources"] })
      void navigate({ to: "/t/$team/integrations/sources", params: { team } })
    },
    onError: (e) => toast.error(e.message),
  })
  if (!source) return null
  return (
    <>
      <PageHeader title="Zone de danger" />
      <SettingsCard title="Zone de danger">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <p className="text-sm font-semibold">Supprimer la source</p>
            <p className="text-sm text-muted-foreground">Refusé si des applications l'utilisent. L'App reste sur GitHub : supprimez-la aussi dans ses réglages.</p>
          </div>
          <Button size="sm" variant="destructive" onClick={() => setOpen(true)}>
            <Trash2Icon /> Supprimer
          </Button>
        </div>
      </SettingsCard>
      <ConfirmDialog open={open} onOpenChange={setOpen} title={`Supprimer « ${source.name} » ?`} confirmText={source.name} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </>
  )
}

/** Carte « Choisir une GitHub App » (nouvelle ressource) : liste des Apps et ajout. */
export function ChooseGitHubApp({ team, value, onChange, returnTo }: { team: string; value: string; onChange: (id: string) => void; returnTo: string }) {
  const apps = (useQuery(sourcesQuery(team)).data ?? []).filter((s) => s.type === "github")
  const [adding, setAdding] = useState(false)
  return (
    <SettingsCard
      title="Choisir une GitHub App"
      description="Choisissez l'installation qui a accès au dépôt à déployer."
      actions={
        <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
          <PlusIcon /> Ajouter une GitHub App
        </Button>
      }
      bodyClassName="p-2"
    >
      {apps.map((s) => {
        const st = sourceState(s)
        const usable = s.installed
        return (
          <button
            key={s.id}
            type="button"
            onClick={() => (usable ? onChange(s.id) : undefined)}
            className={cn("flex w-full items-center gap-3 rounded-xl px-3 py-3 text-left transition-colors", usable ? "hover:bg-foreground/[0.04]" : "opacity-80", value === s.id && "bg-foreground/[0.06]")}
          >
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-muted-foreground">
              <GitBranchIcon className="size-4" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-semibold">{s.name}</span>
              <span className="block truncate text-xs text-muted-foreground">{s.html_url}</span>
            </span>
            {!usable && (
              <Link to="/t/$team/integrations/sources/$source" params={{ team, source: s.id }} className="text-xs text-primary hover:underline">
                Terminer l'installation
              </Link>
            )}
            <Pill label={st.label} dot={st.dot} />
          </button>
        )
      })}
      {apps.length === 0 && <p className="px-3 py-6 text-center text-sm text-muted-foreground">Aucune GitHub App. Ajoutez-en une pour déployer des dépôts privés.</p>}
      <NewGitHubAppDialog team={team} open={adding} onOpenChange={setAdding} returnTo={returnTo} />
    </SettingsCard>
  )
}
