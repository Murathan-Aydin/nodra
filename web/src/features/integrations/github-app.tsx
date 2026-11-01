import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { ArrowRightIcon, ChevronDownIcon, GitBranchIcon, PlusIcon, SettingsIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
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

// Le manifeste préparé par l'API et la page de retour sont gardés le temps de passer par GitHub.
const pendingKey = (id: string) => `nodra-github-app-${id}`
type Pending = { form_action: string; manifest: string; returnTo?: string }
const readPending = (id: string): Pending | null => {
  try {
    return JSON.parse(sessionStorage.getItem(pendingKey(id)) ?? "null") as Pending | null
  } catch {
    return null
  }
}

/** Dialogue « Nouvelle GitHub App » (New GitHub App du fork) : crée la source puis ouvre sa page d'installation. */
export function NewGitHubAppDialog({ team, open, onOpenChange, returnTo }: { team: string; open: boolean; onOpenChange: (o: boolean) => void; returnTo?: string }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [d, setD] = useState(() => ({ name: randomName(), organization: "", is_public: false, html_url: "https://github.com" }))
  const [enterprise, setEnterprise] = useState(false)
  const create = useMutation({
    mutationFn: () => post<{ source: Source; form_action: string; manifest: string }>(`/teams/${team}/sources/github/manifest`, { ...d, organization: d.organization || undefined }),
    onSuccess: (r) => {
      try {
        sessionStorage.setItem(pendingKey(r.source.id), JSON.stringify({ form_action: r.form_action, manifest: r.manifest, returnTo }))
      } catch {
        // stockage indisponible : l'installation manuelle reste possible
      }
      void qc.invalidateQueries({ queryKey: ["team", team, "sources"] })
      onOpenChange(false)
      void navigate({ to: "/t/$team/integrations/sources/$source", params: { team, source: r.source.id } })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Nouvelle GitHub App</DialogTitle>
          <DialogDescription>Une GitHub App donne accès aux dépôts privés, aux webhooks et aux previews des pull requests.</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Nom *">
              <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
            </Field>
            <Field label="Organisation" hint="Vide = votre compte personnel.">
              <Input value={d.organization} onChange={(e) => setD({ ...d, organization: e.target.value })} placeholder="Compte personnel si vide" />
            </Field>
          </div>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={d.is_public} onCheckedChange={(c) => setD({ ...d, is_public: c === true })} /> App publique (installable par d'autres comptes)
          </label>
          <div className="rounded-xl border border-border">
            <button type="button" onClick={() => setEnterprise(!enterprise)} className="flex w-full items-center justify-between px-4 py-3 text-sm font-medium">
              GitHub auto-hébergé / Enterprise
              <ChevronDownIcon className={cn("size-4 transition-transform", enterprise && "rotate-180")} />
            </button>
            {enterprise && (
              <div className="border-t border-border p-4">
                <Field label="Adresse de GitHub *" hint="L'adresse de l'API est déduite (/api/v3 pour Enterprise).">
                  <Input value={d.html_url} onChange={(e) => setD({ ...d, html_url: e.target.value })} className="font-mono" />
                </Field>
              </div>
            )}
          </div>
          <Button className="w-full" disabled={!d.name.trim() || !d.html_url} loading={create.isPending} onClick={() => create.mutate()}>
            Continuer
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

/** Page d'une source (source/github/change du fork) : installation automatique ou manuelle, état, suppression. */
export function SourcePage({ team, id, installed }: { team: string; id: string; installed?: boolean }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const source = useQuery({ queryKey: ["team", team, "source", id], queryFn: () => get<Source>(`/teams/${team}/sources/${id}`) }).data
  const [deleting, setDeleting] = useState(false)
  const pending = readPending(id)
  const remove = useMutation({
    mutationFn: () => api(`/teams/${team}/sources/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["team", team, "sources"] })
      void navigate({ to: "/t/$team/integrations/sources", params: { team } })
    },
    onError: (e) => toast.error(e.message),
  })
  if (!source) return null
  const st = sourceState(source)
  return (
    <>
      <PageHeader
        title={source.name}
        description={source.installed ? `GitHub App installée${source.organization ? ` sur ${source.organization}` : ""}.` : "Terminez l'enregistrement de cette GitHub App avant de l'utiliser comme source."}
        actions={
          <>
            <Pill label={st.label} dot={st.dot} />
            <Button variant="destructive" onClick={() => setDeleting(true)}>
              <Trash2Icon /> Supprimer
            </Button>
          </>
        }
      />
      {source.installed ? (
        <SettingsCard
          title="GitHub App"
          actions={
            pending?.returnTo && (
              <Button asChild>
                <Link to={pending.returnTo}>
                  Continuer la création <ArrowRightIcon />
                </Link>
              </Button>
            )
          }
        >
          {installed && <p className="mb-4 text-sm text-success">Installation terminée.</p>}
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Application">
              <CopyInput value={source.app_slug || String(source.app_id ?? "")} />
            </Field>
            <Field label="Installation">
              <CopyInput value={String(source.installation_id ?? "")} />
            </Field>
            <Field label="Adresse">
              <CopyInput value={source.html_url} />
            </Field>
            <Field label="Gérer l'App sur GitHub">
              <Button variant="outline" asChild>
                <a href={`${source.html_url}/settings/installations`} target="_blank" rel="noreferrer">
                  Ouvrir les installations
                </a>
              </Button>
            </Field>
          </div>
        </SettingsCard>
      ) : source.configured ? (
        <SettingsCard title="Installer l'App" description="L'App est créée : installez-la sur les dépôts à déployer.">
          <Button asChild>
            <a href={`${source.html_url}${source.html_url === "https://github.com" ? "/apps" : "/github-apps"}/${source.app_slug}/installations/new`}>Installer sur GitHub</a>
          </Button>
        </SettingsCard>
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <SettingsCard title="Installation automatique" actions={<Pill label="Recommandée" dot="bg-success" />}>
            <div className="space-y-4">
              <div className="rounded-xl border border-border p-4 text-sm">
                <p className="font-medium">Permissions demandées</p>
                <p className="mt-1 text-muted-foreground">Contenu : lecture · Métadonnées : lecture · E-mails : lecture · Pull requests : écriture (previews) · Administration : lecture</p>
                <p className="mt-2 text-muted-foreground">Événements : push, pull request.</p>
              </div>
              {pending ? (
                <Button className="w-full" onClick={() => submitManifest(pending)}>
                  S'enregistrer sur GitHub
                </Button>
              ) : (
                <p className="text-sm text-muted-foreground">Le lien d'enregistrement a expiré : supprimez cette source et recréez la GitHub App, ou utilisez l'installation manuelle.</p>
              )}
            </div>
          </SettingsCard>
          <ManualInstall team={team} source={source} />
        </div>
      )}
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer « ${source.name} » ?`} description="Refusé si des applications l'utilisent." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </>
  )
}

/** Soumet le manifeste à GitHub (formulaire POST), qui crée l'App puis revient sur l'instance. */
function submitManifest(p: Pending) {
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
      <Input value={d[k]} onChange={(e) => setD({ ...d, [k]: e.target.value })} className="font-mono" type={k.includes("secret") ? "password" : "text"} />
    </Field>
  )
  return (
    <SettingsCard title="Installation manuelle" actions={<Pill label="Avancé" dot="bg-muted-foreground" />}>
      {!open ? (
        <div className="space-y-6">
          <span className="flex size-10 items-center justify-center rounded-xl border border-border bg-muted text-muted-foreground">
            <SettingsIcon className="size-5" />
          </span>
          <p className="text-sm text-muted-foreground">À utiliser si l'enregistrement automatique n'est pas possible ou pour garder la main sur la configuration de l'App (créée vous-même sur GitHub).</p>
          <Button variant="outline" onClick={() => setOpen(true)}>
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
