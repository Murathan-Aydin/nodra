import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { CalendarClockIcon, PencilIcon, PlayIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { StatusBadge } from "@/components/status-badge"
import { TaskDialog } from "@/components/task-output"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { del, get, patch, post, teamPath } from "@/lib/api"
import { relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { ScheduledTask, ServerTask } from "@/lib/types"

type Draft = { id?: string; name: string; command: string; frequency: string; container: string; timeout: number; enabled: boolean }

/** Commandes planifiées (cron) exécutées dans un conteneur de la ressource. */
export function ScheduledPanel({ team, resource }: { team: string; resource: string }) {
  const base = teamPath(team, "/scheduled-tasks")
  const key = ["team", team, "scheduled", resource]
  const tasks = useQuery({ queryKey: key, queryFn: () => get<ScheduledTask[]>(base, { resource_id: resource }) })
  const canEdit = usePermission(team, "resource.terminal", { type: "resource", id: resource })
  const [draft, setDraft] = useState<Draft | null>(null)
  const [deleting, setDeleting] = useState<ScheduledTask | null>(null)
  const [history, setHistory] = useState<ScheduledTask | null>(null)
  const [running, setRunning] = useState<ServerTask | null>(null)
  const save = useApiMutation((d: Draft) => (d.id ? patch(`${base}/${d.id}`, d) : post(base, { ...d, resource_id: resource })), {
    success: "Tâche enregistrée",
    invalidate: [key],
    onSuccess: () => setDraft(null),
  })
  const run = useApiMutation((t: ScheduledTask) => post<ServerTask>(`${base}/${t.id}/run`), { onSuccess: setRunning })
  const remove = useApiMutation((t: ScheduledTask) => del(`${base}/${t.id}`), { success: "Tâche supprimée", invalidate: [key], onSuccess: () => setDeleting(null) })
  const executions = useQuery({
    queryKey: ["team", team, "scheduled-executions", history?.id],
    queryFn: () => get<ServerTask[]>(`${base}/${history?.id}/executions`),
    enabled: !!history,
  })
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">Commandes exécutées selon une expression cron (fuseau horaire du serveur).</p>
        {canEdit && (
          <Button size="sm" onClick={() => setDraft({ name: "", command: "", frequency: "@daily", container: "", timeout: 3600, enabled: true })}>
            <PlusIcon /> Nouvelle tâche
          </Button>
        )}
      </div>
      {!tasks.data?.length ? (
        <EmptyState icon={CalendarClockIcon} title="Aucune tâche planifiée" />
      ) : (
        <Card>
          <ul className="divide-y">
            {tasks.data.map((t) => (
              <li key={t.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
                <span className="min-w-0 flex-1">
                  <button className="block truncate font-medium hover:underline" onClick={() => setHistory(t)}>
                    {t.name}
                  </button>
                  <span className="block truncate font-mono text-xs text-muted-foreground">
                    {t.frequency} · {t.command}
                  </span>
                </span>
                {!t.enabled && <Badge variant="secondary">désactivée</Badge>}
                <span className="hidden text-xs text-muted-foreground sm:inline">{t.last_run_at ? relativeTime(t.last_run_at) : "jamais exécutée"}</span>
                {canEdit && (
                  <>
                    <Button variant="ghost" size="icon-sm" aria-label="Exécuter" onClick={() => run.mutate(t)}>
                      <PlayIcon />
                    </Button>
                    <Button variant="ghost" size="icon-sm" aria-label="Modifier" onClick={() => setDraft(t)}>
                      <PencilIcon />
                    </Button>
                    <Button variant="ghost" size="icon-sm" aria-label="Supprimer" onClick={() => setDeleting(t)}>
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </li>
            ))}
          </ul>
        </Card>
      )}
      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{draft?.id ? "Modifier la tâche" : "Nouvelle tâche planifiée"}</DialogTitle>
          </DialogHeader>
          {draft && (
            <div className="space-y-4">
              <Field label="Nom">
                <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
              </Field>
              <Field label="Commande">
                <Input className="font-mono" placeholder="php artisan schedule:run" value={draft.command} onChange={(e) => setDraft({ ...draft, command: e.target.value })} />
              </Field>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Fréquence (cron)" hint="ex. */5 * * * * ou @daily">
                  <Input className="font-mono" value={draft.frequency} onChange={(e) => setDraft({ ...draft, frequency: e.target.value })} />
                </Field>
                <Field label="Délai max. (s)">
                  <Input type="number" min={10} max={86400} value={draft.timeout} onChange={(e) => setDraft({ ...draft, timeout: Number(e.target.value) })} />
                </Field>
              </div>
              <Field label="Conteneur (service compose)" hint="Requis s'il y a plusieurs conteneurs.">
                <Input className="font-mono" value={draft.container} onChange={(e) => setDraft({ ...draft, container: e.target.value })} />
              </Field>
              <Label className="font-normal">
                <Switch checked={draft.enabled} onCheckedChange={(enabled) => setDraft({ ...draft, enabled })} /> Activée
              </Label>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button loading={save.isPending} onClick={() => draft && save.mutate(draft)}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={!!history} onOpenChange={(o) => !o && setHistory(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Exécutions de « {history?.name} »</DialogTitle>
          </DialogHeader>
          {!executions.data?.length ? (
            <p className="text-sm text-muted-foreground">Aucune exécution.</p>
          ) : (
            <ul className="max-h-[60vh] space-y-3 overflow-y-auto">
              {executions.data.map((e) => (
                <li key={e.id} className="space-y-1.5">
                  <div className="flex items-center gap-2 text-sm">
                    <StatusBadge status={e.status === "running" ? "in_progress" : e.status} />
                    <span className="text-muted-foreground">{relativeTime(e.created_at)}</span>
                  </div>
                  {e.output && <pre className="max-h-40 overflow-auto rounded-md bg-zinc-950 p-2 font-mono text-xs whitespace-pre-wrap text-zinc-100">{e.output}</pre>}
                  {e.error && <p className="text-xs text-destructive">{e.error}</p>}
                </li>
              ))}
            </ul>
          )}
        </DialogContent>
      </Dialog>
      <TaskDialog team={team} task={running} onOpenChange={(o) => !o && setRunning(null)} />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Supprimer « ${deleting?.name} » ?`}
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </div>
  )
}
