import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { BrushCleaningIcon, PackageCheckIcon, PackageIcon, PlusIcon, PowerIcon, Trash2Icon, XIcon } from "lucide-react"
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { TaskOutput } from "@/components/task-output"
import { TaskStatus } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { SaveButton } from "@/features/applications/general"
import { taskLabel } from "@/features/resources/history"
import { SwitchRow } from "@/features/resources/operations"
import { resourcePage } from "@/features/resources/resource"
import { useDraft } from "@/features/resources/use-patch"
import { api, get, post } from "@/lib/api"
import { bytes, dateTime, duration, relativeTime } from "@/lib/format"
import { resourceStatus } from "@/lib/status"
import type { Destination, ResourceSummary, Server, ServerTask } from "@/lib/types"
import { cn } from "@/lib/utils"
import { useServerPatch } from "./general"
import { TaskButton, useServerTask } from "./run-task"

const KIND_LABEL: Record<ResourceSummary["type"], string> = { application: "Application", database: "Base de données", service: "Service" }

/** Onglet « Ressources » : ressources hébergées et domaines du serveur. */
export function ServerResources({ team, server }: { team: string; server: string }) {
  const list = useQuery({ queryKey: ["team", team, "server", server, "resources"], queryFn: () => get<ResourceSummary[]>(`/teams/${team}/servers/${server}/resources`) })
  const domains = useQuery({ queryKey: ["team", team, "server", server, "domains"], queryFn: () => get<{ domain: string; resource_id: string; type: string; name: string }[]>(`/teams/${team}/servers/${server}/domains`) })
  return (
    <div className="space-y-6">
      <SettingsCard title="Ressources" bodyClassName="p-0">
        {list.data?.map((r) => {
          const s = resourceStatus(r.status)
          return (
            <Link key={r.id} to={resourcePage({ team, kind: r.type, id: r.id })} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0 hover:bg-foreground/[0.03]">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-semibold">{r.name}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {KIND_LABEL[r.type]} · {r.project_name} / {r.environment_name}
                </p>
              </div>
              <Pill label={s.label} dot={s.dot} />
            </Link>
          )
        })}
        {list.data?.length === 0 && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune ressource sur ce serveur.</p>}
      </SettingsCard>
      {!!domains.data?.length && (
        <SettingsCard title="Domaines" bodyClassName="p-0">
          {domains.data.map((d) => (
            <div key={d.domain + d.resource_id} className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0">
              <a href={/^https?:/.test(d.domain) ? d.domain : `https://${d.domain}`} target="_blank" rel="noreferrer" className="min-w-0 flex-1 truncate font-mono text-sm text-primary hover:underline">
                {d.domain}
              </a>
              <span className="text-xs text-muted-foreground">{d.name}</span>
            </div>
          ))}
        </SettingsCard>
      )}
    </div>
  )
}

/** Onglet « Commande » : commande ponctuelle sur le serveur (journalisée). */
export function ServerCommand({ team, server }: { team: string; server: string }) {
  const [command, setCommand] = useState("")
  const task = useServerTask(team, server)
  return (
    <SettingsCard title="Commande ponctuelle" description="Exécutée par SSH avec l'utilisateur du serveur ; enregistrée dans le journal d'audit.">
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (command.trim()) task.run.mutate({ path: "/commands", json: { command } })
        }}
      >
        <Input value={command} onChange={(e) => setCommand(e.target.value)} placeholder="df -h" className="font-mono" />
        <Button type="submit" size="sm" loading={task.run.isPending} disabled={!command.trim()}>
          Exécuter
        </Button>
      </form>
      {task.output && <div className="mt-4">{task.output}</div>}
    </SettingsCard>
  )
}

/** Onglet « Destinations » : réseaux Docker du serveur. */
export function ServerDestinations({ team, server, destinations }: { team: string; server: string; destinations: Destination[] }) {
  const qc = useQueryClient()
  const [d, setD] = useState({ name: "", network: "" })
  const task = useServerTask(team, server)
  const [deleting, setDeleting] = useState<Destination | null>(null)
  const remove = useMutation({
    mutationFn: (dest: Destination) => api(`/teams/${team}/servers/${server}/destinations/${dest.id}`, { method: "DELETE" }),
    onSuccess: () => {
      setDeleting(null)
      void qc.invalidateQueries({ queryKey: ["team", team, "server", server] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Destinations" description="Réseaux Docker où les ressources sont déployées. La destination par défaut ne peut pas être supprimée." bodyClassName="p-0">
      {destinations.map((dest) => (
        <div key={dest.id} className="flex items-center gap-3 border-b border-border px-4 py-3">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{dest.name}</p>
            <p className="truncate font-mono text-xs text-muted-foreground">{dest.network}</p>
          </div>
          {dest.is_default ? (
            <Pill label="Par défaut" dot="bg-primary" />
          ) : (
            <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(dest)}>
              <Trash2Icon />
            </Button>
          )}
        </div>
      ))}
      <form
        className="flex flex-wrap items-end gap-3 p-4"
        onSubmit={(e) => {
          e.preventDefault()
          task.run.mutate({ path: "/destinations", json: { name: d.name || undefined, network: d.network } })
        }}
      >
        <Field label="Nom">
          <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
        </Field>
        <Field label="Réseau Docker">
          <Input value={d.network} onChange={(e) => setD({ ...d, network: e.target.value })} className="font-mono" required />
        </Field>
        <Button type="submit" size="sm" variant="outline" loading={task.run.isPending}>
          <PlusIcon /> Créer
        </Button>
      </form>
      {task.output && <div className="px-4 pb-4">{task.output}</div>}
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={`Supprimer ${deleting?.name} ?`} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => deleting && remove.mutate(deleting)} />
    </SettingsCard>
  )
}

/** Onglet « Nettoyage Docker » : planification, seuil et nettoyage immédiat. */
export function ServerCleanup({ team, server }: { team: string; server: Server }) {
  const s = server.settings ?? {}
  const { draft: d, set, changed, dirty } = useDraft({
    docker_cleanup_frequency: String(s.docker_cleanup_frequency ?? "0 0 * * *"),
    docker_cleanup_threshold: String(s.docker_cleanup_threshold ?? 80),
    delete_unused_volumes: Boolean(s.delete_unused_volumes),
    delete_unused_networks: Boolean(s.delete_unused_networks),
  })
  const save = useServerPatch(team, server.id)
  const task = useServerTask(team, server.id)
  return (
    <SettingsCard
      title="Nettoyage Docker"
      description="Conteneurs arrêtés, images inutilisées et cache de build ; tout ce que Nodra gère est épargné."
      actions={
        <>
          <TaskButton label="Nettoyer maintenant" icon={<BrushCleaningIcon />} pending={task.run.isPending} onClick={() => task.run.mutate({ path: "/docker-cleanup" })} />
          <SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ settings: { ...changed, ...(changed.docker_cleanup_threshold ? { docker_cleanup_threshold: Number(changed.docker_cleanup_threshold) } : {}) } })} />
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Fréquence (cron)" hint="Ex. « 0 0 * * * » ou « @daily ».">
          <Input value={d.docker_cleanup_frequency} onChange={(e) => set("docker_cleanup_frequency", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Seuil disque (%)" hint="Nettoyage déclenché au-delà (vérifié toutes les 5 min).">
          <Input value={d.docker_cleanup_threshold} onChange={(e) => set("docker_cleanup_threshold", e.target.value.replace(/\D/g, ""))} />
        </Field>
      </div>
      <SwitchRow label="Supprimer les volumes inutilisés" hint="Attention : données perdues." checked={d.delete_unused_volumes} onChange={(v) => set("delete_unused_volumes", v)} />
      <SwitchRow label="Supprimer les réseaux inutilisés" checked={d.delete_unused_networks} onChange={(v) => set("delete_unused_networks", v)} />
      {task.output && <div className="mt-4">{task.output}</div>}
    </SettingsCard>
  )
}

type Drain = { enabled: boolean; type?: "newrelic" | "axiom" | "custom"; config?: Record<string, string>; updated_at?: string }

/** Onglet « Log drains » : envoi des logs des conteneurs vers New Relic, Axiom ou Fluent Bit. */
export function ServerLogDrains({ team, server }: { team: string; server: string }) {
  const drain = useQuery({ queryKey: ["team", team, "server", server, "log-drain"], queryFn: () => get<Drain>(`/teams/${team}/servers/${server}/log-drain`) })
  if (!drain.data) return null
  return <LogDrainForm key={drain.data.updated_at ?? "none"} team={team} server={server} drain={drain.data} />
}

function LogDrainForm({ team, server, drain }: { team: string; server: string; drain: Drain }) {
  const qc = useQueryClient()
  const [type, setType] = useState<NonNullable<Drain["type"]>>(drain.type ?? "newrelic")
  const [c, setC] = useState<Record<string, string>>(drain.config ?? {})
  const start = useServerTask(team, server)
  const stop = useServerTask(team, server)
  const field = (k: string, label: string, secret?: boolean, hint?: string) => (
    <Field label={label} hint={hint}>
      <Input value={c[k] ?? ""} onChange={(e) => setC({ ...c, [k]: e.target.value })} type={secret ? "password" : "text"} className="font-mono" />
    </Field>
  )
  return (
    <SettingsCard
      title="Log drain"
      description="Conteneur nodra-log-drain (Fluent Bit). Chaque ressource l'active dans ses réglages ; appliqué au prochain déploiement."
      actions={
        <>
          <Pill label={drain.enabled ? "Actif" : "Inactif"} dot={drain.enabled ? "bg-success" : "bg-muted-foreground"} />
          {drain.enabled && <TaskButton label="Arrêter" pending={stop.run.isPending} onClick={() => stop.run.mutate({ path: "/log-drain", method: "DELETE" })} />}
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Destination">
          <Select value={type} onValueChange={(v) => setType(v as typeof type)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="newrelic">New Relic</SelectItem>
              <SelectItem value="axiom">Axiom</SelectItem>
              <SelectItem value="custom">Fluent Bit personnalisé</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <div />
        {type === "newrelic" && (
          <>
            {field("license_key", "Clé de licence", true)}
            {field("base_uri", "URI", false, "https://log-api.eu.newrelic.com/log/v1")}
          </>
        )}
        {type === "axiom" && (
          <>
            {field("axiom_dataset", "Dataset")}
            {field("axiom_api_key", "Clé API", true)}
            {field("axiom_domain", "Domaine", false, "api.axiom.co par défaut")}
          </>
        )}
        {type === "custom" && (
          <>
            <Field label="fluent-bit.conf" className="sm:col-span-2">
              <Textarea value={c.custom_config ?? ""} onChange={(e) => setC({ ...c, custom_config: e.target.value })} rows={10} className="font-mono" />
            </Field>
            <Field label="parsers.conf (facultatif)" className="sm:col-span-2">
              <Textarea value={c.custom_parsers ?? ""} onChange={(e) => setC({ ...c, custom_parsers: e.target.value })} rows={5} className="font-mono" />
            </Field>
          </>
        )}
      </div>
      <Button
        size="sm"
        className="mt-4"
        loading={start.run.isPending}
        onClick={() =>
          start.run.mutate(
            { path: "/log-drain", method: "PUT", json: { ...c, type } },
            { onSuccess: () => void qc.invalidateQueries({ queryKey: ["team", team, "server", server, "log-drain"] }) },
          )
        }
      >
        {drain.enabled ? "Enregistrer et redémarrer" : "Activer"}
      </Button>
      {start.output && <div className="mt-4">{start.output}</div>}
      {stop.output && <div className="mt-4">{stop.output}</div>}
    </SettingsCard>
  )
}

type Metric = { ts: string; cpu_percent: number; mem_used: number; mem_total: number; disk_used: number; disk_total: number; load1: number }

const RANGES = [
  { value: "1", label: "Dernière heure" },
  { value: "6", label: "6 heures" },
  { value: "24", label: "24 heures" },
  { value: "168", label: "7 jours" },
]

/** Onglet « Métriques » : processeur, mémoire, disque et charge (recharts). */
export function ServerMetrics({ team, server }: { team: string; server: string }) {
  const [range, setRange] = useState("1")
  const metrics = useQuery({
    queryKey: ["team", team, "server", server, "metrics", range],
    queryFn: () => get<Metric[]>(`/teams/${team}/servers/${server}/metrics?since=${encodeURIComponent(new Date(Date.now() - Number(range) * 3600_000).toISOString())}`),
    refetchInterval: 60_000,
  })
  const data = (metrics.data ?? []).map((m) => ({
    t: new Date(m.ts).toLocaleTimeString("fr", { hour: "2-digit", minute: "2-digit" }),
    cpu: Math.round(m.cpu_percent * 10) / 10,
    mem: m.mem_total ? Math.round((m.mem_used / m.mem_total) * 1000) / 10 : 0,
    disk: m.disk_total ? Math.round((m.disk_used / m.disk_total) * 1000) / 10 : 0,
    load: m.load1,
  }))
  const last = metrics.data?.at(-1)
  return (
    <SettingsCard
      title="Métriques"
      description={last ? `Mémoire ${bytes(last.mem_used)} / ${bytes(last.mem_total)} · disque ${bytes(last.disk_used)} / ${bytes(last.disk_total)}` : undefined}
      actions={
        <Select value={range} onValueChange={setRange}>
          <SelectTrigger className="h-8 w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RANGES.map((r) => (
              <SelectItem key={r.value} value={r.value}>
                {r.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      }
    >
      {data.length === 0 ? (
        <p className="text-sm text-muted-foreground">{metrics.isPending ? "Chargement…" : "Aucune mesure sur la période."}</p>
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Chart title="Processeur (%)" data={data} k="cpu" color="var(--primary)" max={100} />
          <Chart title="Mémoire (%)" data={data} k="mem" color="var(--success)" max={100} />
          <Chart title="Disque (%)" data={data} k="disk" color="var(--warning)" max={100} />
          <Chart title="Charge (1 min)" data={data} k="load" color="var(--muted-foreground)" />
        </div>
      )}
    </SettingsCard>
  )
}

function Chart({ title, data, k, color, max }: { title: string; data: Record<string, number | string>[]; k: string; color: string; max?: number }) {
  return (
    <div>
      <p className="mb-2 text-xs font-medium text-muted-foreground">{title}</p>
      <ResponsiveContainer width="100%" height={180}>
        <AreaChart data={data} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
          <XAxis dataKey="t" tick={{ fontSize: 10, fill: "var(--muted-foreground)" }} minTickGap={40} />
          <YAxis domain={[0, max ?? "auto"]} tick={{ fontSize: 10, fill: "var(--muted-foreground)" }} />
          <Tooltip contentStyle={{ background: "var(--popover)", border: "1px solid var(--border)", borderRadius: 8, fontSize: 12 }} />
          <Area type="monotone" dataKey={k} stroke={color} fill={color} fillOpacity={0.15} strokeWidth={1.5} isAnimationActive={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

/** Onglet « Mises à jour » : paquets système disponibles et mise à jour. */
export function ServerPackages({ team, server }: { team: string; server: string }) {
  const check = useServerTask(team, server)
  const update = useServerTask(team, server)
  const [confirm, setConfirm] = useState(false)
  return (
    <SettingsCard
      title="Mises à jour du système"
      actions={
        <>
          <TaskButton label="Vérifier" icon={<PackageCheckIcon />} pending={check.run.isPending} onClick={() => check.run.mutate({ path: "/packages/check" })} />
          <TaskButton label="Mettre à jour" icon={<PackageIcon />} variant="default" pending={update.run.isPending} onClick={() => setConfirm(true)} />
        </>
      }
    >
      <p className="text-sm text-muted-foreground">Liste et installe les mises à jour des paquets du serveur (apt, dnf, apk, pacman).</p>
      {check.output && <div className="mt-4">{check.output}</div>}
      {update.output && <div className="mt-4">{update.output}</div>}
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title="Mettre à jour les paquets ?"
        description="Certains services du serveur peuvent redémarrer pendant l'opération."
        destructive={false}
        actionLabel="Mettre à jour"
        onConfirm={() => {
          setConfirm(false)
          update.run.mutate({ path: "/packages/update" })
        }}
      />
    </SettingsCard>
  )
}

/** Onglet « Tâches » : historique des tâches du serveur, sortie en direct, annulation. */
export function ServerTasks({ team, server }: { team: string; server: string }) {
  const qc = useQueryClient()
  const tasks = useQuery({ queryKey: ["team", team, "server", server, "tasks"], queryFn: () => get<ServerTask[]>(`/teams/${team}/servers/${server}/tasks`), refetchInterval: 10_000 })
  const [open, setOpen] = useState<string | null>(null)
  const cancel = useMutation({
    mutationFn: (id: string) => post(`/teams/${team}/servers/${server}/tasks/${id}/cancel`),
    onSuccess: () => {
      toast.success("Annulation demandée")
      void qc.invalidateQueries({ queryKey: ["team", team, "server", server, "tasks"] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Tâches" description="50 dernières tâches du serveur." bodyClassName="p-0">
      {tasks.data?.map((t) => (
        <div key={t.id} className="border-b border-border last:border-b-0">
          <div className="flex items-center gap-3 px-4 py-3">
            <button type="button" onClick={() => setOpen(open === t.id ? null : t.id)} className="min-w-0 flex-1 text-left">
              <p className="truncate text-sm font-medium">{taskLabel(t.kind)}</p>
              <p className="truncate text-xs text-muted-foreground" title={dateTime(t.created_at)}>
                {relativeTime(t.created_at)}
                {t.started_at && ` · ${duration(t.started_at, t.finished_at)}`}
                {t.error && ` · ${t.error}`}
              </p>
            </button>
            <TaskStatus status={t.status} />
            {(t.status === "queued" || t.status === "running") && (
              <Button size="sm" variant="ghost" onClick={() => cancel.mutate(t.id)}>
                <XIcon /> Annuler
              </Button>
            )}
          </div>
          {open === t.id && (
            <div className={cn("px-4 pb-4")}>
              <TaskOutput team={team} server={server} task={t.id} />
            </div>
          )}
        </div>
      ))}
      {tasks.data?.length === 0 && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune tâche.</p>}
    </SettingsCard>
  )
}

/** Onglet « Zone de danger » : redémarrage et suppression du serveur. */
export function ServerDanger({ team, server }: { team: string; server: Server }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const reboot = useServerTask(team, server.id)
  const [rebooting, setRebooting] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const remove = useMutation({
    mutationFn: () => api<{ approval_request?: unknown } | undefined>(`/teams/${team}/servers/${server.id}`, { method: "DELETE" }),
    onSuccess: (r) => {
      setDeleting(false)
      if (r?.approval_request) return void toast.info("Suppression en attente d'approbation")
      toast.success("Serveur supprimé")
      void qc.invalidateQueries({ queryKey: ["team", team] })
      void navigate({ to: `/t/${team}/servers` })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Zone de danger">
      <div className="space-y-5">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <p className="text-sm font-semibold">Redémarrer le serveur</p>
            <p className="text-sm text-muted-foreground">Les ressources sont indisponibles pendant le redémarrage.</p>
          </div>
          <Button size="sm" variant="outline" onClick={() => setRebooting(true)}>
            <PowerIcon /> Redémarrer
          </Button>
        </div>
        {reboot.output}
        <div className="flex flex-wrap items-center justify-between gap-4 border-t border-border pt-5">
          <div>
            <p className="text-sm font-semibold">Supprimer le serveur</p>
            <p className="text-sm text-muted-foreground">Impossible tant que des ressources y sont déployées.</p>
          </div>
          <Button size="sm" variant="destructive" onClick={() => setDeleting(true)} disabled={server.is_localhost}>
            Supprimer
          </Button>
        </div>
      </div>
      <ConfirmDialog
        open={rebooting}
        onOpenChange={setRebooting}
        title="Redémarrer le serveur ?"
        actionLabel="Redémarrer"
        onConfirm={() => {
          setRebooting(false)
          reboot.run.mutate({ path: "/reboot" })
        }}
      />
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer « ${server.name} » ?`} confirmText={server.name} actionLabel="Supprimer définitivement" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </SettingsCard>
  )
}
