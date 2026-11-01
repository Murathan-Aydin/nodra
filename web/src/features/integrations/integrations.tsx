import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { CheckCircle2Icon, PlusIcon, RefreshCwIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { EmptyPanel, PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { api, get, post } from "@/lib/api"
import { Link } from "@tanstack/react-router"
import { NewGitHubAppDialog, sourceState, type Source as GHSource } from "./github-app"
import { relativeTime } from "@/lib/format"

type Source = { id: string; type: "github" | "gitlab" | "gitea"; name: string; html_url: string; api_url: string; organization: string; is_public: boolean; app_id: number | null; app_slug: string; installation_id: number | null; configured: boolean; installed: boolean; has_access_token: boolean; created_at: string }

const SOURCE_LABEL = { github: "GitHub App", gitlab: "GitLab", gitea: "Gitea / Forgejo" }

/** Page « Intégrations › Sources » (Sources du fork) : GitHub App (création automatique), GitLab, Gitea. */
export function SourcesPage({ team }: { team: string }) {
  const qc = useQueryClient()
  const key = ["team", team, "sources"]
  const sources = useQuery({ queryKey: key, queryFn: () => get<Source[]>(`/teams/${team}/sources`) }).data ?? []
  const [adding, setAdding] = useState<"" | "github" | "gitlab" | "gitea">("")
  const [deleting, setDeleting] = useState<Source | null>(null)
  const remove = useMutation({
    mutationFn: (s: Source) => api(`/teams/${team}/sources/${s.id}`, { method: "DELETE" }),
    onSuccess: () => (setDeleting(null), void qc.invalidateQueries({ queryKey: key })),
    onError: (e) => toast.error(e.message),
  })
  return (
    <>
      <PageHeader
        title="Sources Git"
        description="Accès aux dépôts privés et webhooks automatiques ; les dépôts publics n'en ont pas besoin."
        actions={
          <Select value={adding} onValueChange={(v) => setAdding(v as typeof adding)}>
            <SelectTrigger className="h-8 w-48">
              <SelectValue placeholder="Ajouter une source" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="github">GitHub App</SelectItem>
              <SelectItem value="gitlab">GitLab</SelectItem>
              <SelectItem value="gitea">Gitea / Forgejo</SelectItem>
            </SelectContent>
          </Select>
        }
      />
      <div className="space-y-4">
        <NewGitHubAppDialog team={team} open={adding === "github"} onOpenChange={(o) => !o && setAdding("")} />
        {(adding === "gitlab" || adding === "gitea") && <TokenSource key={adding} team={team} type={adding} onDone={() => setAdding("")} />}
        {sources.map((s) => (
          <SettingsCard
            key={s.id}
            title={s.name}
            description={`${SOURCE_LABEL[s.type]} · ${s.html_url}${s.organization ? ` · ${s.organization}` : ""}`}
            actions={
              <>
                <Pill {...sourceState(s as GHSource)} />
                {s.type === "github" && (
                  <Button size="sm" variant="outline" asChild>
                    <Link to="/t/$team/integrations/sources/$source" params={{ team, source: s.id }}>
                      {s.installed ? "Ouvrir" : "Terminer l'installation"}
                    </Link>
                  </Button>
                )}
                <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(s)}>
                  <Trash2Icon />
                </Button>
              </>
            }
            bodyClassName="px-5 py-3"
          >
            <p className="text-xs text-muted-foreground">
              Ajoutée {relativeTime(s.created_at)}
              {s.app_id && ` · App n° ${s.app_id}`}
              {s.installation_id && ` · installation ${s.installation_id}`}
            </p>
          </SettingsCard>
        ))}
        {!adding && sources.length === 0 && <EmptyPanel title="Aucune source Git" />}
      </div>
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={`Supprimer « ${deleting?.name} » ?`} description="Refusé si des applications l'utilisent." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => deleting && remove.mutate(deleting)} />
    </>
  )
}

function TokenSource({ team, type, onDone }: { team: string; type: "gitlab" | "gitea"; onDone: () => void }) {
  const qc = useQueryClient()
  const [d, setD] = useState({ name: "", access_token: "", html_url: type === "gitlab" ? "https://gitlab.com" : "" })
  const add = useMutation({
    mutationFn: () => post(`/teams/${team}/sources`, { type, ...d }),
    onSuccess: () => (toast.success("Source ajoutée"), void qc.invalidateQueries({ queryKey: ["team", team, "sources"] }), onDone()),
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title={`Nouvelle source ${SOURCE_LABEL[type]}`}
      description={type === "gitlab" ? "Jeton d'accès avec read_api et read_repository." : "Jeton d'accès avec lecture des dépôts."}
      actions={
        <>
          <Button size="sm" variant="ghost" onClick={onDone}>
            Annuler
          </Button>
          <Button size="sm" disabled={!d.name || !d.access_token || !d.html_url} loading={add.isPending} onClick={() => add.mutate()}>
            Ajouter
          </Button>
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Nom *">
          <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
        </Field>
        <Field label="Adresse *">
          <Input value={d.html_url} onChange={(e) => setD({ ...d, html_url: e.target.value })} className="font-mono" placeholder="https://git.exemple.fr" />
        </Field>
        <Field label="Jeton *">
          <Input type="password" value={d.access_token} onChange={(e) => setD({ ...d, access_token: e.target.value })} className="font-mono" />
        </Field>
      </div>
    </SettingsCard>
  )
}

type Registry = { id: string; name: string; host: string; username: string; created_at: string }

/** Page « Intégrations › Registres » : identifiants des registres Docker privés. */
export function RegistriesPage({ team }: { team: string }) {
  const qc = useQueryClient()
  const key = ["team", team, "registries"]
  const list = useQuery({ queryKey: key, queryFn: () => get<Registry[]>(`/teams/${team}/docker-registries`) }).data ?? []
  const [d, setD] = useState({ name: "", host: "ghcr.io", username: "", password: "" })
  const [deleting, setDeleting] = useState<Registry | null>(null)
  const add = useMutation({
    mutationFn: () => post(`/teams/${team}/docker-registries`, d),
    onSuccess: () => (setD({ name: "", host: "ghcr.io", username: "", password: "" }), toast.success("Registre ajouté"), void qc.invalidateQueries({ queryKey: key })),
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: (r: Registry) => api(`/teams/${team}/docker-registries/${r.id}`, { method: "DELETE" }),
    onSuccess: () => (setDeleting(null), void qc.invalidateQueries({ queryKey: key })),
    onError: (e) => toast.error(e.message),
  })
  return (
    <>
      <PageHeader title="Registres Docker" description="Identifiants utilisés pour tirer et construire les images ; hérités par les sous-équipes." />
      <SettingsCard title="Registres" bodyClassName="p-0">
        {list.map((r) => (
          <div key={r.id} className="flex items-center gap-3 border-b border-border px-4 py-3">
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">{r.name}</p>
              <p className="truncate font-mono text-xs text-muted-foreground">
                {r.username}@{r.host}
              </p>
            </div>
            <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(r)}>
              <Trash2Icon />
            </Button>
          </div>
        ))}
        <form
          className="grid gap-3 p-4 sm:grid-cols-[1fr_1fr_1fr_1fr_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            add.mutate()
          }}
        >
          <Field label="Nom">
            <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} required />
          </Field>
          <Field label="Hôte">
            <Input value={d.host} onChange={(e) => setD({ ...d, host: e.target.value })} className="font-mono" required />
          </Field>
          <Field label="Utilisateur">
            <Input value={d.username} onChange={(e) => setD({ ...d, username: e.target.value })} required />
          </Field>
          <Field label="Mot de passe / jeton">
            <Input type="password" value={d.password} onChange={(e) => setD({ ...d, password: e.target.value })} required />
          </Field>
          <Button type="submit" size="sm" loading={add.isPending}>
            <PlusIcon /> Ajouter
          </Button>
        </form>
      </SettingsCard>
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={`Supprimer « ${deleting?.name} » ?`} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => deleting && remove.mutate(deleting)} />
    </>
  )
}

type S3 = { id: string; name: string; description: string; endpoint: string; bucket: string; region: string; path_prefix: string; is_usable: boolean; last_error: string }

/** Page « Intégrations › Stockages S3 » (S3 Storage du fork). */
export function S3Page({ team }: { team: string }) {
  const key = ["team", team, "s3-storages"]
  const list = useQuery({ queryKey: key, queryFn: () => get<S3[]>(`/teams/${team}/s3-storages`) }).data ?? []
  const [adding, setAdding] = useState(false)
  return (
    <>
      <PageHeader
        title="Stockages S3"
        description="Destinations des sauvegardes : AWS, Cloudflare R2, MinIO, Garage… L'accès au bucket est vérifié à l'enregistrement."
        actions={
          <Button size="sm" onClick={() => setAdding(true)}>
            <PlusIcon /> Ajouter
          </Button>
        }
      />
      <div className="space-y-4">
        {adding && <S3Card team={team} queryKey={key} onDone={() => setAdding(false)} />}
        {list.map((s) => (
          <S3Card key={s.id + s.is_usable + s.name} team={team} queryKey={key} s3={s} />
        ))}
        {!adding && list.length === 0 && <EmptyPanel title="Aucun stockage S3" />}
      </div>
    </>
  )
}

function S3Card({ team, queryKey, s3, onDone }: { team: string; queryKey: unknown[]; s3?: S3; onDone?: () => void }) {
  const qc = useQueryClient()
  const [d, setD] = useState({ name: s3?.name ?? "", description: s3?.description ?? "", endpoint: s3?.endpoint ?? "", bucket: s3?.bucket ?? "", region: s3?.region ?? "us-east-1", path_prefix: s3?.path_prefix ?? "", access_key: "", secret_key: "" })
  const [deleting, setDeleting] = useState(false)
  const refresh = () => void qc.invalidateQueries({ queryKey })
  const save = useMutation({
    mutationFn: () => (s3 ? api(`/teams/${team}/s3-storages/${s3.id}`, { method: "PATCH", json: d }) : post(`/teams/${team}/s3-storages`, d)),
    onSuccess: () => (toast.success("Stockage enregistré"), refresh(), onDone?.()),
    onError: (e) => toast.error(e.message),
  })
  const check = useMutation({ mutationFn: () => post(`/teams/${team}/s3-storages/${s3!.id}/check`), onSuccess: refresh, onError: (e) => toast.error(e.message) })
  const remove = useMutation({ mutationFn: () => api(`/teams/${team}/s3-storages/${s3!.id}`, { method: "DELETE" }), onSuccess: refresh, onError: (e) => toast.error(e.message) })
  const f = (k: keyof typeof d, label: string, extra: { secret?: boolean; hint?: string } = {}) => (
    <Field label={label} hint={extra.hint}>
      <Input value={d[k]} onChange={(e) => setD({ ...d, [k]: e.target.value })} type={extra.secret ? "password" : "text"} className="font-mono" />
    </Field>
  )
  return (
    <SettingsCard
      title={s3 ? s3.name : "Nouveau stockage S3"}
      description={s3?.last_error || undefined}
      actions={
        <>
          {s3 && <Pill label={s3.is_usable ? "Accessible" : "Inaccessible"} dot={s3.is_usable ? "bg-success" : "bg-destructive"} />}
          {s3 && (
            <Button size="sm" variant="ghost" loading={check.isPending} onClick={() => check.mutate()}>
              <RefreshCwIcon /> Vérifier
            </Button>
          )}
          {s3 ? (
            <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
              <Trash2Icon />
            </Button>
          ) : (
            <Button size="sm" variant="ghost" onClick={onDone}>
              Annuler
            </Button>
          )}
          <Button size="sm" variant="outline" loading={save.isPending} onClick={() => save.mutate()}>
            <CheckCircle2Icon /> Enregistrer
          </Button>
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-3">
        {f("name", "Nom *")}
        {f("endpoint", "Endpoint *", { hint: "https://s3.eu-west-3.amazonaws.com" })}
        {f("bucket", "Bucket *")}
        {f("region", "Région")}
        {f("path_prefix", "Préfixe", { hint: "Dossier dans le bucket." })}
        {f("description", "Description")}
        {f("access_key", "Clé d'accès", { secret: true, hint: s3 ? "Vide = inchangée." : undefined })}
        {f("secret_key", "Clé secrète", { secret: true, hint: s3 ? "Vide = inchangée." : undefined })}
      </div>
      {s3 && <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer « ${s3.name} » ?`} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />}
    </SettingsCard>
  )
}

