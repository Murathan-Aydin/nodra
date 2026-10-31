import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ArchiveIcon, CloudIcon, DownloadIcon, HardDriveIcon, PencilIcon, PlayIcon, PlusIcon, RotateCcwIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { StatusBadge } from "@/components/status-badge"
import { TaskDialog } from "@/components/task-output"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { API_BASE, ApiError, del, get, patch, post, teamPath } from "@/lib/api"
import { bytes, dateTime, relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { BackupExecution, BackupSchedule, S3Storage, ServerTask } from "@/lib/types"

type Draft = Omit<BackupSchedule, "id" | "team_id" | "resource_id" | "last_run_at" | "created_at"> & { id?: string }

const PRESETS: [string, string][] = [
  ["0 * * * *", "Toutes les heures"],
  ["0 0 * * *", "Chaque jour à minuit"],
  ["0 3 * * 0", "Chaque dimanche à 3 h"],
  ["0 0 1 * *", "Le 1er de chaque mois"],
]

const NEW: Draft = {
  kind: "database",
  volume_name: "",
  databases: "",
  dump_all: false,
  frequency: "0 0 * * *",
  enabled: true,
  keep_local: 7,
  keep_local_days: 0,
  local_backup: true,
  s3_storage_id: null,
  keep_s3: 30,
  keep_s3_days: 0,
}

/**
 * Sauvegardes planifiées d'une ressource et leurs exécutions (téléchargement, restauration, suppression).
 * `database` : la ressource est une base dont le type sait se sauvegarder ; sinon seuls les volumes sont proposés.
 * Les archives de volumes (.tar.gz) sont toujours restaurables ; les dumps seulement si `restorable`.
 */
export function BackupsPanel({
  team,
  resource,
  database,
  restorable,
  dumpAll,
}: {
  team: string
  resource: string
  database: boolean
  /** Restauration automatique des dumps possible (bases hors Redis/KeyDB). */
  restorable?: boolean
  dumpAll?: boolean
}) {
  const scope = { type: "resource", id: resource }
  const canManage = usePermission(team, "resource.backup.manage", scope)
  const canRestore = usePermission(team, "resource.backup.restore", scope)
  const schedKey = ["team", team, "backups", resource]
  const execKey = ["team", team, "backup-executions", resource]
  const schedules = useQuery({ queryKey: schedKey, queryFn: () => get<BackupSchedule[]>(teamPath(team, "/backups"), { resource_id: resource }) })
  const executions = useQuery({
    queryKey: execKey,
    queryFn: () => get<BackupExecution[]>(teamPath(team, "/backup-executions"), { resource_id: resource }),
    refetchInterval: (q) => (q.state.data?.some((e) => e.status === "running" || e.status === "queued") ? 3000 : false),
  })
  const s3 = useQuery({ queryKey: ["team", team, "s3-storages"], queryFn: () => get<S3Storage[]>(teamPath(team, "/s3-storages")), retry: false })
  const s3Name = (id: string | null) => s3.data?.find((s) => s.id === id)?.name ?? "S3"
  const [draft, setDraft] = useState<Draft | null>(null)
  const [deleting, setDeleting] = useState<BackupSchedule | null>(null)
  const [restoring, setRestoring] = useState<BackupExecution | null>(null)
  const [removing, setRemoving] = useState<BackupExecution | null>(null)
  const [task, setTask] = useState<ServerTask | null>(null)
  const invalidate = [schedKey, execKey]

  const saveDraft = useApiMutation(
    (d: Draft) => {
      const body = {
        volume_name: d.volume_name,
        databases: d.databases,
        dump_all: d.dump_all,
        frequency: d.frequency,
        enabled: d.enabled,
        keep_local: d.keep_local,
        keep_local_days: d.keep_local_days,
        local_backup: d.local_backup,
        s3_storage_id: d.s3_storage_id ?? "", // "" retire le stockage S3
        keep_s3: d.keep_s3,
        keep_s3_days: d.keep_s3_days,
      }
      return d.id ? patch(teamPath(team, `/backups/${d.id}`), body) : post(teamPath(team, "/backups"), { ...body, kind: d.kind, resource_id: resource })
    },
    { success: "Planification enregistrée", invalidate, onSuccess: () => setDraft(null) },
  )
  const toggle = useApiMutation((s: BackupSchedule) => patch(teamPath(team, `/backups/${s.id}`), { enabled: !s.enabled }), { invalidate })
  const run = useApiMutation((id: string) => post<ServerTask>(teamPath(team, `/backups/${id}/run`)), {
    invalidate,
    onSuccess: (t) => t && "kind" in t && setTask(t),
  })
  const remove = useApiMutation((id: string) => del(teamPath(team, `/backups/${id}`)), {
    success: "Planification supprimée",
    invalidate,
    onSuccess: () => setDeleting(null),
  })
  const restore = useApiMutation((id: string) => post<ServerTask>(teamPath(team, `/backup-executions/${id}/restore`)), {
    invalidate,
    onSuccess: (t) => {
      setRestoring(null)
      if (t && "kind" in t) setTask(t)
    },
  })
  const removeExec = useApiMutation((id: string) => del(teamPath(team, `/backup-executions/${id}`)), {
    success: "Sauvegarde supprimée",
    invalidate,
    onSuccess: () => setRemoving(null),
  })

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex-row items-start justify-between gap-4">
          <div className="space-y-1.5">
            <CardTitle>Planifications</CardTitle>
            <CardDescription>Fréquence cron dans le fuseau horaire du serveur ; rétention appliquée après chaque sauvegarde.</CardDescription>
          </div>
          {canManage && (
            <Button size="sm" onClick={() => setDraft({ ...NEW, kind: database ? "database" : "volume" })}>
              <PlusIcon /> Planifier
            </Button>
          )}
        </CardHeader>
        <CardContent>
          {!schedules.data?.length ? (
            <EmptyState icon={ArchiveIcon} title="Aucune sauvegarde planifiée" />
          ) : (
            <ul className="divide-y rounded-lg border">
              {schedules.data.map((s) => (
                <li key={s.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5 text-sm">
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                      <code className="font-mono">{s.frequency}</code>
                      <span className="text-muted-foreground">
                        {s.kind === "volume" ? `volume ${s.volume_name}` : s.dump_all ? "toutes les bases" : s.databases || "base par défaut"}
                      </span>
                    </span>
                    <span className="flex flex-wrap gap-x-3 text-xs text-muted-foreground">
                      {s.local_backup && (
                        <span className="inline-flex items-center gap-1">
                          <HardDriveIcon className="size-3" /> local, {s.keep_local || "∞"} gardées
                        </span>
                      )}
                      {s.s3_storage_id && (
                        <span className="inline-flex items-center gap-1">
                          <CloudIcon className="size-3" /> {s3Name(s.s3_storage_id)}, {s.keep_s3 || "∞"} gardées
                        </span>
                      )}
                      <span>dernière : {relativeTime(s.last_run_at)}</span>
                    </span>
                  </span>
                  <Switch checked={s.enabled} onCheckedChange={() => toggle.mutate(s)} disabled={!canManage} aria-label="Activée" />
                  {canManage && (
                    <>
                      <Button variant="ghost" size="icon-sm" aria-label="Sauvegarder maintenant" onClick={() => run.mutate(s.id)}>
                        <PlayIcon />
                      </Button>
                      <Button variant="ghost" size="icon-sm" aria-label="Modifier" onClick={() => setDraft({ ...s })}>
                        <PencilIcon />
                      </Button>
                      <Button variant="ghost" size="icon-sm" aria-label="Supprimer" onClick={() => setDeleting(s)}>
                        <Trash2Icon />
                      </Button>
                    </>
                  )}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Sauvegardes</CardTitle>
        </CardHeader>
        <CardContent>
          {!executions.data?.length ? (
            <p className="text-sm text-muted-foreground">Aucune sauvegarde pour l'instant.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Statut</TableHead>
                  <TableHead>Fichier</TableHead>
                  <TableHead>Emplacement</TableHead>
                  <TableHead>Date</TableHead>
                  <TableHead className="text-right">Taille</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {executions.data.map((e) => (
                  <TableRow key={e.id}>
                    <TableCell>
                      <StatusBadge status={e.status} />
                    </TableCell>
                    <TableCell className="max-w-72">
                      <span className="block truncate font-mono text-xs" title={e.filename}>
                        {e.filename.split("/").pop() || "—"}
                      </span>
                      {e.message && <span className="block truncate text-xs text-muted-foreground" title={e.message}>{e.message}</span>}
                    </TableCell>
                    <TableCell className="space-x-1">
                      {!e.local_deleted && e.filename && <Badge variant="outline">local</Badge>}
                      {e.s3_key && !e.s3_deleted && <Badge variant="outline">{s3Name(e.s3_storage_id)}</Badge>}
                    </TableCell>
                    <TableCell className="text-muted-foreground" title={dateTime(e.created_at)}>
                      {relativeTime(e.created_at)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{e.size ? bytes(e.size) : "—"}</TableCell>
                    <TableCell className="text-right whitespace-nowrap">
                      {e.status === "success" && (
                        <>
                          {canManage && (
                            <Button variant="ghost" size="icon-sm" aria-label="Télécharger" onClick={() => void downloadBackup(team, e.id)}>
                              <DownloadIcon />
                            </Button>
                          )}
                          {canRestore && (restorable || e.filename.endsWith(".tar.gz")) && (
                            <Button variant="ghost" size="icon-sm" aria-label="Restaurer" onClick={() => setRestoring(e)}>
                              <RotateCcwIcon />
                            </Button>
                          )}
                        </>
                      )}
                      {canManage && (
                        <Button variant="ghost" size="icon-sm" aria-label="Supprimer" onClick={() => setRemoving(e)}>
                          <Trash2Icon />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{draft?.id ? "Modifier la planification" : "Nouvelle sauvegarde planifiée"}</DialogTitle>
          </DialogHeader>
          {draft && (
            <div className="grid gap-4 sm:grid-cols-2">
              {database && !draft.id && (
                <Field label="Contenu" className="sm:col-span-2">
                  <Select value={draft.kind} onValueChange={(v) => setDraft({ ...draft, kind: v as Draft["kind"] })}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="database">Dump de la base</SelectItem>
                      <SelectItem value="volume">Archive d'un volume Docker</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              )}
              {draft.kind === "volume" ? (
                <Field label="Volume" hint="Volume Docker de la ressource (onglet Stockages)." className="sm:col-span-2">
                  <Input className="font-mono" value={draft.volume_name} onChange={(e) => setDraft({ ...draft, volume_name: e.target.value })} disabled={!!draft.id} />
                </Field>
              ) : (
                <>
                  <Field label="Bases" hint="Séparées par des virgules ; vide = base par défaut.">
                    <Input className="font-mono" value={draft.databases} onChange={(e) => setDraft({ ...draft, databases: e.target.value })} disabled={draft.dump_all} />
                  </Field>
                  {dumpAll && (
                    <Label className="flex items-center gap-2 self-end pb-2 font-normal">
                      <Switch checked={draft.dump_all} onCheckedChange={(v) => setDraft({ ...draft, dump_all: v })} /> Toutes les bases
                    </Label>
                  )}
                </>
              )}
              <Field label="Fréquence (cron)">
                <Input className="font-mono" value={draft.frequency} onChange={(e) => setDraft({ ...draft, frequency: e.target.value })} />
              </Field>
              <Field label="Raccourcis">
                <Select value="" onValueChange={(v) => setDraft({ ...draft, frequency: v })}>
                  <SelectTrigger>
                    <SelectValue placeholder="Choisir…" />
                  </SelectTrigger>
                  <SelectContent>
                    {PRESETS.map(([cron, label]) => (
                      <SelectItem key={cron} value={cron}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Label className="flex items-center gap-2 font-normal sm:col-span-2">
                <Switch checked={draft.local_backup} onCheckedChange={(v) => setDraft({ ...draft, local_backup: v })} /> Garder une copie sur le serveur
              </Label>
              {draft.local_backup && (
                <>
                  <Field label="Copies locales gardées" hint="0 = sans limite.">
                    <Input type="number" min={0} value={draft.keep_local} onChange={(e) => setDraft({ ...draft, keep_local: Number(e.target.value) })} />
                  </Field>
                  <Field label="Âge maximal (jours)" hint="0 = sans limite.">
                    <Input type="number" min={0} value={draft.keep_local_days} onChange={(e) => setDraft({ ...draft, keep_local_days: Number(e.target.value) })} />
                  </Field>
                </>
              )}
              <Field label="Stockage S3" className="sm:col-span-2">
                <Select value={draft.s3_storage_id ?? "none"} onValueChange={(v) => setDraft({ ...draft, s3_storage_id: v === "none" ? null : v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">Aucun</SelectItem>
                    {s3.data?.map((s) => (
                      <SelectItem key={s.id} value={s.id} disabled={!s.is_usable}>
                        {s.name} ({s.bucket}){!s.is_usable && " — inaccessible"}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              {draft.s3_storage_id && (
                <>
                  <Field label="Copies S3 gardées" hint="0 = sans limite.">
                    <Input type="number" min={0} value={draft.keep_s3} onChange={(e) => setDraft({ ...draft, keep_s3: Number(e.target.value) })} />
                  </Field>
                  <Field label="Âge maximal (jours)">
                    <Input type="number" min={0} value={draft.keep_s3_days} onChange={(e) => setDraft({ ...draft, keep_s3_days: Number(e.target.value) })} />
                  </Field>
                </>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button loading={saveDraft.isPending} onClick={() => draft && saveDraft.mutate(draft)}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="Supprimer cette planification ?"
        description="Les sauvegardes déjà faites sont conservées."
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
      <ConfirmDialog
        open={!!restoring}
        onOpenChange={(o) => !o && setRestoring(null)}
        title="Restaurer cette sauvegarde ?"
        description="Les données actuelles seront écrasées par celles de la sauvegarde."
        confirmLabel="Restaurer"
        confirmText="restaurer"
        destructive
        loading={restore.isPending}
        onConfirm={() => restoring && restore.mutate(restoring.id)}
      />
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title="Supprimer cette sauvegarde ?"
        description="Le fichier local et la copie S3 sont supprimés."
        confirmLabel="Supprimer"
        destructive
        loading={removeExec.isPending}
        onConfirm={() => removing && removeExec.mutate(removing.id)}
      />
      <TaskDialog team={team} task={task} onOpenChange={(o) => !o && setTask(null)} />
    </div>
  )
}

/** Lien S3 présigné (JSON) ou fichier servi par Nodra : dans ce cas le navigateur le télécharge lui-même. */
async function downloadBackup(team: string, id: string) {
  const url = `${API_BASE}${teamPath(team, `/backup-executions/${id}/download`)}`
  try {
    const res = await fetch(url, { credentials: "include" })
    if (!res.ok) {
      const body = (await res.json().catch(() => ({}))) as { error?: string }
      throw new ApiError(res.status, body.error ?? "Téléchargement impossible")
    }
    if (res.headers.get("Content-Type")?.includes("application/json")) {
      window.location.href = ((await res.json()) as { url: string }).url
      return
    }
    await res.body?.cancel()
    window.location.href = url
  } catch (e) {
    toast.error(e instanceof ApiError ? e.message : "Téléchargement impossible")
  }
}
