import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { DownloadIcon, PlayIcon, PlusIcon, RotateCcwIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { api, get, post } from "@/lib/api"
import { bytes, dateTime, duration, relativeTime } from "@/lib/format"
import { downloadUrl } from "@/lib/format"
import { SwitchRow } from "./operations"
import type { ResourceRef } from "./resource"

export interface BackupSchedule {
  id: string
  kind: "database" | "volume"
  resource_id: string
  volume_name: string
  databases: string
  dump_all: boolean
  frequency: string
  enabled: boolean
  keep_local: number
  keep_local_days: number
  local_backup: boolean
  s3_storage_id: string | null
  keep_s3: number
  keep_s3_days: number
}

interface Execution {
  id: string
  schedule_id: string
  database_name: string
  status: "running" | "success" | "failed"
  filename: string
  size: number
  s3_key: string
  local_deleted: boolean
  s3_deleted: boolean
  message: string
  created_at: string
  finished_at: string | null
}

const STATUS: Record<Execution["status"], { label: string; dot: string }> = {
  running: { label: "En cours", dot: "bg-primary animate-pulse" },
  success: { label: "Réussie", dot: "bg-success" },
  failed: { label: "Échouée", dot: "bg-destructive" },
}

/** Onglet « Sauvegardes » (backups du fork) : planifications et exécutions, restauration et téléchargement. */
export function BackupsTab({ resource, kind = "database", volumes }: { resource: ResourceRef; kind?: "database" | "volume"; volumes?: string[] }) {
  const t = resource.team
  const schedKey = ["team", t, "backups", resource.id]
  const schedules = useQuery({ queryKey: schedKey, queryFn: () => get<BackupSchedule[]>(`/teams/${t}/backups?resource_id=${resource.id}`) })
  const execs = useQuery({ queryKey: [...schedKey, "executions"], queryFn: () => get<Execution[]>(`/teams/${t}/backup-executions?resource_id=${resource.id}`), refetchInterval: 10_000 })
  const [adding, setAdding] = useState(false)
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Planifications"
        description="Lancées chaque minute à échéance (fuseau du serveur) ; rétention locale et S3 appliquée après chaque exécution."
        actions={
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            <PlusIcon /> Ajouter
          </Button>
        }
        bodyClassName="p-0"
      >
        {adding && (
          <div className="border-b border-border p-5">
            <ScheduleForm resource={resource} kind={kind} volumes={volumes} queryKey={schedKey} onDone={() => setAdding(false)} />
          </div>
        )}
        {schedules.isPending ? (
          <Skeleton className="m-4 h-16" />
        ) : schedules.data?.length ? (
          schedules.data.map((s) => <ScheduleRow key={s.id} resource={resource} schedule={s} volumes={volumes} queryKey={schedKey} />)
        ) : (
          !adding && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune sauvegarde planifiée.</p>
        )}
      </SettingsCard>
      <SettingsCard title="Exécutions" bodyClassName="p-0">
        {execs.data?.length ? (
          execs.data.map((e) => <ExecutionRow key={e.id} team={t} execution={e} queryKey={[...schedKey, "executions"]} />)
        ) : (
          <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune sauvegarde pour l'instant.</p>
        )}
      </SettingsCard>
    </div>
  )
}

function useS3(team: string) {
  return useQuery({ queryKey: ["team", team, "s3-storages"], queryFn: () => get<{ id: string; name: string }[]>(`/teams/${team}/s3-storages`) }).data ?? []
}

function ScheduleForm({ resource, kind, volumes, schedule, queryKey, onDone }: { resource: ResourceRef; kind: "database" | "volume"; volumes?: string[]; schedule?: BackupSchedule; queryKey: unknown[]; onDone: () => void }) {
  const qc = useQueryClient()
  const s3 = useS3(resource.team)
  const [d, setD] = useState({
    frequency: schedule?.frequency ?? "0 0 * * *",
    enabled: schedule?.enabled ?? true,
    databases: schedule?.databases ?? "",
    dump_all: schedule?.dump_all ?? false,
    volume_name: schedule?.volume_name ?? volumes?.[0] ?? "",
    keep_local: String(schedule?.keep_local ?? 7),
    keep_local_days: String(schedule?.keep_local_days ?? 0),
    local_backup: schedule?.local_backup ?? true,
    s3_storage_id: schedule?.s3_storage_id ?? "",
    keep_s3: String(schedule?.keep_s3 ?? 30),
    keep_s3_days: String(schedule?.keep_s3_days ?? 0),
  })
  const body = {
    frequency: d.frequency,
    enabled: d.enabled,
    ...(kind === "database" ? { databases: d.databases, dump_all: d.dump_all } : { volume_name: d.volume_name }),
    keep_local: Number(d.keep_local),
    keep_local_days: Number(d.keep_local_days),
    local_backup: d.local_backup,
    s3_storage_id: d.s3_storage_id || null,
    keep_s3: Number(d.keep_s3),
    keep_s3_days: Number(d.keep_s3_days),
  }
  const save = useMutation({
    mutationFn: () => (schedule ? api(`/teams/${resource.team}/backups/${schedule.id}`, { method: "PATCH", json: body }) : post(`/teams/${resource.team}/backups`, { ...body, kind, resource_id: resource.id })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey })
      toast.success("Planification enregistrée")
      onDone()
    },
    onError: (e) => toast.error(e.message),
  })
  const num = (k: keyof typeof d) => (e: React.ChangeEvent<HTMLInputElement>) => setD({ ...d, [k]: e.target.value.replace(/\D/g, "") })
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Fréquence (cron)" hint="Ex. « 0 0 * * * » : chaque nuit à minuit.">
          <Input value={d.frequency} onChange={(e) => setD({ ...d, frequency: e.target.value })} className="font-mono" />
        </Field>
        {kind === "database" ? (
          <Field label="Bases à sauvegarder" hint="Séparées par des virgules ; vide = la base initiale.">
            <Input value={d.databases} onChange={(e) => setD({ ...d, databases: e.target.value })} className="font-mono" disabled={d.dump_all} />
          </Field>
        ) : (
          <Field label="Volume">
            <Select value={d.volume_name} onValueChange={(v) => setD({ ...d, volume_name: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(volumes ?? []).map((v) => (
                  <SelectItem key={v} value={v}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        )}
        <Field label="Copies locales gardées" hint="0 = sans limite de nombre.">
          <Input value={d.keep_local} onChange={num("keep_local")} />
        </Field>
        <Field label="Jours locaux gardés" hint="0 = sans limite d'âge.">
          <Input value={d.keep_local_days} onChange={num("keep_local_days")} />
        </Field>
        <Field label="Stockage S3">
          <Select value={d.s3_storage_id || "none"} onValueChange={(v) => setD({ ...d, s3_storage_id: v === "none" ? "" : v })}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">Aucun</SelectItem>
              {s3.map((s) => (
                <SelectItem key={s.id} value={s.id}>
                  {s.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {d.s3_storage_id && (
          <div className="grid grid-cols-2 gap-4">
            <Field label="Copies S3 gardées">
              <Input value={d.keep_s3} onChange={num("keep_s3")} />
            </Field>
            <Field label="Jours S3 gardés">
              <Input value={d.keep_s3_days} onChange={num("keep_s3_days")} />
            </Field>
          </div>
        )}
      </div>
      <div>
        {kind === "database" && <SwitchRow label="Toutes les bases" hint="pg_dumpall pour PostgreSQL, --all-databases pour MySQL/MariaDB." checked={d.dump_all} onChange={(v) => setD({ ...d, dump_all: v })} />}
        <SwitchRow label="Garder une copie sur le serveur" checked={d.local_backup} onChange={(v) => setD({ ...d, local_backup: v })} />
        <SwitchRow label="Activée" checked={d.enabled} onChange={(v) => setD({ ...d, enabled: v })} />
      </div>
      <div className="flex gap-2">
        <Button type="submit" size="sm" loading={save.isPending}>
          Enregistrer
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDone}>
          Annuler
        </Button>
      </div>
    </form>
  )
}

function ScheduleRow({ resource, schedule, volumes, queryKey }: { resource: ResourceRef; schedule: BackupSchedule; volumes?: string[]; queryKey: unknown[] }) {
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const run = useMutation({
    mutationFn: () => post(`/teams/${resource.team}/backups/${schedule.id}/run`),
    onSuccess: () => {
      toast.success("Sauvegarde lancée")
      void qc.invalidateQueries({ queryKey: [...queryKey, "executions"] })
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api(`/teams/${resource.team}/backups/${schedule.id}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey }),
    onError: (e) => toast.error(e.message),
  })
  if (editing)
    return (
      <div className="border-b border-border p-5 last:border-b-0">
        <ScheduleForm resource={resource} kind={schedule.kind} volumes={volumes} schedule={schedule} queryKey={queryKey} onDone={() => setEditing(false)} />
      </div>
    )
  return (
    <div className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-sm">{schedule.frequency}</p>
        <p className="truncate text-xs text-muted-foreground">
          {schedule.kind === "volume" ? `Volume ${schedule.volume_name}` : schedule.dump_all ? "Toutes les bases" : schedule.databases || "Base initiale"}
          {` · ${schedule.keep_local} copie(s) locale(s)`}
          {schedule.s3_storage_id && ` · S3 (${schedule.keep_s3})`}
          {!schedule.enabled && " · désactivée"}
        </p>
      </div>
      <Button size="sm" variant="outline" loading={run.isPending} onClick={() => run.mutate()}>
        <PlayIcon /> Sauvegarder maintenant
      </Button>
      <Button size="sm" variant="ghost" onClick={() => setEditing(true)}>
        Modifier
      </Button>
      <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
        <Trash2Icon />
      </Button>
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title="Supprimer cette planification ?" description="Les sauvegardes déjà faites sont conservées." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </div>
  )
}

function ExecutionRow({ team, execution: e, queryKey }: { team: string; execution: Execution; queryKey: unknown[] }) {
  const qc = useQueryClient()
  const [restoring, setRestoring] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const s = STATUS[e.status] ?? STATUS.running
  const download = useMutation({
    mutationFn: async () => {
      const path = `/api/v1/teams/${team}/backup-executions/${e.id}/download`
      const res = await fetch(path, { credentials: "include", headers: { Accept: "application/json" } })
      if (!res.ok) throw new Error(`Erreur HTTP ${res.status}`)
      // Sur S3 : URL présignée en JSON ; sinon le fichier lui-même.
      if (res.headers.get("content-type")?.includes("application/json")) return downloadUrl(((await res.json()) as { url: string }).url, e.filename)
      downloadUrl(path, e.filename)
    },
    onError: (err) => toast.error(err.message),
  })
  const restore = useMutation({
    mutationFn: () => post<{ approval_request?: unknown }>(`/teams/${team}/backup-executions/${e.id}/restore`),
    onSuccess: (r) => {
      setRestoring(false)
      toast.success(r?.approval_request ? "Restauration en attente d'approbation" : "Restauration lancée")
    },
    onError: (err) => toast.error(err.message),
  })
  const remove = useMutation({
    mutationFn: () => api(`/teams/${team}/backup-executions/${e.id}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey }),
    onError: (err) => toast.error(err.message),
  })
  return (
    <div className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-sm">{e.filename || e.database_name || "—"}</p>
        <p className="truncate text-xs text-muted-foreground" title={dateTime(e.created_at)}>
          {relativeTime(e.created_at)} · {duration(e.created_at, e.finished_at)} · {bytes(e.size)}
          {e.s3_key && !e.s3_deleted && " · S3"}
          {e.local_deleted && " · copie locale supprimée"}
          {e.message && e.status === "failed" && ` · ${e.message}`}
        </p>
      </div>
      <Pill label={s.label} dot={s.dot} />
      {e.status === "success" && (
        <>
          <Button size="sm" variant="ghost" loading={download.isPending} onClick={() => download.mutate()}>
            <DownloadIcon /> Télécharger
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setRestoring(true)}>
            <RotateCcwIcon /> Restaurer
          </Button>
        </>
      )}
      <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
        <Trash2Icon />
      </Button>
      <ConfirmDialog
        open={restoring}
        onOpenChange={setRestoring}
        title="Restaurer cette sauvegarde ?"
        description="Les données actuelles seront remplacées par celles de la sauvegarde."
        actionLabel="Restaurer"
        loading={restore.isPending}
        onConfirm={() => restore.mutate()}
      />
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title="Supprimer cette sauvegarde ?" description="Le fichier local et l'objet S3 sont supprimés." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </div>
  )
}
