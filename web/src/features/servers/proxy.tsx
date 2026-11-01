import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { PlayIcon, PlusIcon, RotateCwIcon, SquareIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { CodeEditor } from "@/components/code-editor"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { SaveButton } from "@/features/applications/general"
import { useDraft } from "@/features/resources/use-patch"
import { api, get, post } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { resourceStatus } from "@/lib/status"
import { TaskButton, useServerTask } from "./run-task"

type Proxy = { type: "traefik" | "caddy" | "none"; status: string; acme_email: string; compose: string; is_custom: boolean; default_compose: string }

const proxyKey = (team: string, server: string) => ["team", team, "server", server, "proxy"]

/** Proxy courant du serveur. */
export function useProxy(team: string, server: string) {
  return useQuery({ queryKey: proxyKey(team, server), queryFn: () => get<Proxy>(`/teams/${team}/servers/${server}/proxy`), refetchInterval: 15_000 })
}

/** Onglet « Proxy › Configuration » : type, e-mail ACME, compose et démarrage. */
export function ProxyConfig({ team, server }: { team: string; server: string }) {
  const proxy = useProxy(team, server).data
  const task = useServerTask(team, server)
  if (!proxy) return null
  const st = resourceStatus(proxy.status)
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Proxy"
        description="Conteneur nodra-proxy sur le réseau nodra, ports 80/443, certificats Let's Encrypt."
        actions={
          <>
            <Pill label={st.label} dot={st.dot} />
            {proxy.type !== "none" &&
              (proxy.status.startsWith("running") ? (
                <>
                  <TaskButton label="Redémarrer" icon={<RotateCwIcon />} pending={task.run.isPending} onClick={() => task.run.mutate({ path: "/proxy/restart" })} />
                  <TaskButton label="Arrêter" icon={<SquareIcon />} pending={task.run.isPending} onClick={() => task.run.mutate({ path: "/proxy/stop" })} />
                </>
              ) : (
                <TaskButton label="Démarrer" icon={<PlayIcon />} variant="default" pending={task.run.isPending} onClick={() => task.run.mutate({ path: "/proxy/start" })} />
              ))}
          </>
        }
      >
        <ProxySettings key={proxy.type + proxy.acme_email} team={team} server={server} proxy={proxy} />
        {task.output && <div className="mt-4">{task.output}</div>}
      </SettingsCard>
      {proxy.type !== "none" && <ProxyCompose key={proxy.compose} team={team} server={server} proxy={proxy} />}
    </div>
  )
}

function usePatchProxy(team: string, server: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (fields: Partial<Proxy>) => api(`/teams/${team}/servers/${server}/proxy`, { method: "PATCH", json: fields }),
    onSuccess: () => {
      toast.success("Proxy enregistré")
      void qc.invalidateQueries({ queryKey: proxyKey(team, server) })
    },
    onError: (e) => toast.error(e.message),
  })
}

function ProxySettings({ team, server, proxy }: { team: string; server: string; proxy: Proxy }) {
  const { draft: d, set, changed, dirty } = useDraft({ type: proxy.type, acme_email: proxy.acme_email })
  const save = usePatchProxy(team, server)
  return (
    <div className="grid gap-4 sm:grid-cols-[14rem_1fr_auto] sm:items-end">
      <Field label="Type" hint="Changer de type exige un proxy arrêté.">
        <Select value={d.type} onValueChange={(v) => set("type", v as Proxy["type"])}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="traefik">Traefik</SelectItem>
            <SelectItem value="caddy">Caddy</SelectItem>
            <SelectItem value="none">Aucun</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field label="E-mail Let's Encrypt">
        <Input value={d.acme_email} onChange={(e) => set("acme_email", e.target.value)} placeholder="admin@exemple.fr" />
      </Field>
      <SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(changed)} />
    </div>
  )
}

function ProxyCompose({ team, server, proxy }: { team: string; server: string; proxy: Proxy }) {
  const [compose, setCompose] = useState(proxy.compose || proxy.default_compose)
  const save = usePatchProxy(team, server)
  return (
    <SettingsCard
      title="docker-compose du proxy"
      description={proxy.is_custom ? "Configuration personnalisée (redémarrer le proxy pour l'appliquer)." : "Configuration générée par Nodra."}
      actions={
        <>
          {proxy.is_custom && (
            <Button size="sm" variant="ghost" loading={save.isPending} onClick={() => save.mutate({ compose: "" })}>
              Revenir à la configuration générée
            </Button>
          )}
          <SaveButton dirty={compose !== (proxy.compose || proxy.default_compose)} pending={save.isPending} onClick={() => save.mutate({ compose })} />
        </>
      }
    >
      <CodeEditor value={compose} onChange={setCompose} language="yaml" height="420px" />
    </SettingsCard>
  )
}

type DynConfig = { id: string; name: string; content: string; updated_at: string }

/** Onglet « Configurations dynamiques » : fichiers .yml (Traefik) ou .caddy (Caddy), synchronisés sans redémarrage. */
export function ProxyDynamic({ team, server }: { team: string; server: string }) {
  const key = ["team", team, "server", server, "proxy-configs"]
  const configs = useQuery({ queryKey: key, queryFn: () => get<DynConfig[]>(`/teams/${team}/servers/${server}/proxy/configs`) })
  const proxy = useProxy(team, server).data
  const [adding, setAdding] = useState(false)
  const ext = proxy?.type === "caddy" ? ".caddy" : ".yml"
  return (
    <SettingsCard
      title="Configurations dynamiques"
      description={`Fichiers ${ext} chargés par le proxy ; le préfixe « nodra- » est réservé.`}
      actions={
        <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
          <PlusIcon /> Ajouter
        </Button>
      }
      bodyClassName="p-0"
    >
      {adding && <DynEditor team={team} server={server} queryKey={key} config={{ id: "", name: `ma-config${ext}`, content: "", updated_at: "" }} isNew onDone={() => setAdding(false)} />}
      {configs.data?.map((c) => <DynEditor key={c.id + c.updated_at} team={team} server={server} queryKey={key} config={c} />)}
      {!adding && !configs.data?.length && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune configuration.</p>}
    </SettingsCard>
  )
}

function DynEditor({ team, server, queryKey, config, isNew, onDone }: { team: string; server: string; queryKey: unknown[]; config: DynConfig; isNew?: boolean; onDone?: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState(config.name)
  const [content, setContent] = useState(config.content)
  const [open, setOpen] = useState(!!isNew)
  const [deleting, setDeleting] = useState(false)
  const nodra = config.name.startsWith("nodra-")
  const save = useMutation({
    mutationFn: () => api(`/teams/${team}/servers/${server}/proxy/configs/${encodeURIComponent(name)}`, { method: "PUT", json: { content } }),
    onSuccess: () => {
      toast.success("Configuration enregistrée")
      void qc.invalidateQueries({ queryKey })
      onDone?.()
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api(`/teams/${team}/servers/${server}/proxy/configs/${encodeURIComponent(config.name)}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey }),
    onError: (e) => toast.error(e.message),
  })
  return (
    <div className="border-b border-border last:border-b-0">
      <div className="flex items-center gap-3 px-4 py-3">
        {isNew ? (
          <Input value={name} onChange={(e) => setName(e.target.value)} className="max-w-sm font-mono" />
        ) : (
          <button type="button" onClick={() => setOpen(!open)} className="min-w-0 flex-1 text-left">
            <p className="truncate font-mono text-sm">{config.name}</p>
            <p className="text-xs text-muted-foreground">
              {nodra ? "Générée par Nodra · " : ""}
              {relativeTime(config.updated_at)}
            </p>
          </button>
        )}
        {isNew && <span className="flex-1" />}
        {open && !nodra && (
          <Button size="sm" variant="outline" loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
        )}
        {isNew ? (
          <Button size="sm" variant="ghost" onClick={onDone}>
            Annuler
          </Button>
        ) : (
          !nodra && (
            <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
              <Trash2Icon />
            </Button>
          )
        )}
      </div>
      {open && (
        <div className="px-4 pb-4">
          <CodeEditor value={content} onChange={setContent} language={name.endsWith(".caddy") ? undefined : "yaml"} height="280px" readOnly={nodra} />
        </div>
      )}
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer ${config.name} ?`} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </div>
  )
}

type Cert = { id: string; name: string; domains: string[] | null; certificate: string; expires_at: string }

/** Onglet « Certificats TLS » : certificats personnalisés chargés par le proxy. */
export function ProxyCertificates({ team, server }: { team: string; server: string }) {
  const qc = useQueryClient()
  const key = ["team", team, "server", server, "proxy-certs"]
  const certs = useQuery({ queryKey: key, queryFn: () => get<Cert[]>(`/teams/${team}/servers/${server}/proxy/certificates`) })
  const [adding, setAdding] = useState(false)
  const [d, setD] = useState({ name: "", certificate: "", private_key: "" })
  const [deleting, setDeleting] = useState<Cert | null>(null)
  const add = useMutation({
    mutationFn: () => post(`/teams/${team}/servers/${server}/proxy/certificates`, d),
    onSuccess: () => {
      toast.success("Certificat ajouté")
      setAdding(false)
      setD({ name: "", certificate: "", private_key: "" })
      void qc.invalidateQueries({ queryKey: key })
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: (c: Cert) => api(`/teams/${team}/servers/${server}/proxy/certificates/${c.id}`, { method: "DELETE" }),
    onSuccess: () => {
      setDeleting(null)
      void qc.invalidateQueries({ queryKey: key })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title="Certificats TLS"
      description="Pour les domaines que Let's Encrypt ne peut pas valider (réseau privé, certificat d'entreprise)."
      actions={
        <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
          <PlusIcon /> Ajouter
        </Button>
      }
      bodyClassName="p-0"
    >
      {adding && (
        <form
          className="grid gap-4 border-b border-border p-5"
          onSubmit={(e) => {
            e.preventDefault()
            add.mutate()
          }}
        >
          <Field label="Nom">
            <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} required />
          </Field>
          <Field label="Certificat (PEM, chaîne possible)">
            <Textarea value={d.certificate} onChange={(e) => setD({ ...d, certificate: e.target.value })} rows={6} className="font-mono" required />
          </Field>
          <Field label="Clé privée (PEM)">
            <Textarea value={d.private_key} onChange={(e) => setD({ ...d, private_key: e.target.value })} rows={6} className="font-mono" required />
          </Field>
          <div className="flex gap-2">
            <Button type="submit" size="sm" loading={add.isPending}>
              Ajouter
            </Button>
            <Button type="button" size="sm" variant="ghost" onClick={() => setAdding(false)}>
              Annuler
            </Button>
          </div>
        </form>
      )}
      {certs.data?.map((c) => (
        <div key={c.id} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{c.name}</p>
            <p className="truncate text-xs text-muted-foreground">
              {(c.domains ?? []).join(", ")} · expire le {dateTime(c.expires_at)}
            </p>
          </div>
          <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(c)}>
            <Trash2Icon />
          </Button>
        </div>
      ))}
      {!adding && !certs.data?.length && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucun certificat personnalisé.</p>}
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={`Supprimer ${deleting?.name} ?`} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => deleting && remove.mutate(deleting)} />
    </SettingsCard>
  )
}
