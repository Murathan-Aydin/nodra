import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { PlayIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { api, get, post } from "@/lib/api"
import { relativeTime } from "@/lib/format"
import type { ServerTask } from "@/lib/types"
import { TaskRow } from "./history"
import type { ResourceRef } from "./resource"

export interface ScheduledTask {
  id: string
  resource_id: string
  name: string
  command: string
  frequency: string
  container: string
  timeout: number
  enabled: boolean
  last_run_at: string | null
}

type Draft = Pick<ScheduledTask, "name" | "command" | "frequency" | "container" | "timeout" | "enabled">
const EMPTY: Draft = { name: "", command: "", frequency: "0 * * * *", container: "", timeout: 3600, enabled: true }

/** Onglet « Tâches planifiées » (scheduled-tasks du fork) : commandes cron dans un conteneur de la ressource. */
export function ScheduledTasksTab({ resource, containers }: { resource: ResourceRef; containers?: { value: string; label: string }[] }) {
  const key = ["team", resource.team, "scheduled-tasks", resource.id]
  const tasks = useQuery({ queryKey: key, queryFn: () => get<ScheduledTask[]>(`/teams/${resource.team}/scheduled-tasks?resource_id=${resource.id}`) })
  const [adding, setAdding] = useState(false)
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Tâches planifiées"
        description="Commandes exécutées selon une expression cron (fuseau du serveur) dans un conteneur en cours d'exécution."
        actions={
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            <PlusIcon /> Ajouter
          </Button>
        }
        bodyClassName="p-0"
      >
        {adding && (
          <div className="border-b border-border p-5">
            <TaskForm resource={resource} containers={containers} queryKey={key} onDone={() => setAdding(false)} />
          </div>
        )}
        {tasks.isPending ? (
          <Skeleton className="m-4 h-20" />
        ) : tasks.data?.length ? (
          tasks.data.map((t) => <TaskItem key={t.id} resource={resource} task={t} containers={containers} queryKey={key} />)
        ) : (
          !adding && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune tâche planifiée.</p>
        )}
      </SettingsCard>
    </div>
  )
}

function TaskForm({ resource, task, containers, queryKey, onDone }: { resource: ResourceRef; task?: ScheduledTask; containers?: { value: string; label: string }[]; queryKey: unknown[]; onDone: () => void }) {
  const qc = useQueryClient()
  const [d, setD] = useState<Draft>(task ?? { ...EMPTY, container: containers?.[0]?.value ?? "" })
  const save = useMutation({
    mutationFn: () =>
      task
        ? api(`/teams/${resource.team}/scheduled-tasks/${task.id}`, { method: "PATCH", json: d })
        : post(`/teams/${resource.team}/scheduled-tasks`, { ...d, resource_id: resource.id, container: d.container || undefined }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey })
      toast.success("Tâche enregistrée")
      onDone()
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <form
      className="grid gap-4 sm:grid-cols-2"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <Field label="Nom">
        <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} required />
      </Field>
      <Field label="Fréquence (cron)" hint="Ex. « 0 3 * * * » : tous les jours à 3 h. Aussi : @hourly, @daily, @weekly.">
        <Input value={d.frequency} onChange={(e) => setD({ ...d, frequency: e.target.value })} className="font-mono" required />
      </Field>
      <Field label="Commande" className="sm:col-span-2">
        <Input value={d.command} onChange={(e) => setD({ ...d, command: e.target.value })} className="font-mono" placeholder="php artisan schedule:run" required />
      </Field>
      {containers && containers.length > 1 && (
        <Field label="Conteneur">
          <Select value={d.container} onValueChange={(v) => setD({ ...d, container: v })}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {containers.map((c) => (
                <SelectItem key={c.value} value={c.value}>
                  {c.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}
      <Field label="Délai maximal (secondes)">
        <Input type="number" min={10} max={86400} value={d.timeout} onChange={(e) => setD({ ...d, timeout: Number(e.target.value) })} />
      </Field>
      <label className="flex items-center gap-2 text-sm sm:col-span-2">
        <Switch checked={d.enabled} onCheckedChange={(v) => setD({ ...d, enabled: v })} /> Activée
      </label>
      <div className="flex gap-2 sm:col-span-2">
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

function TaskItem({ resource, task, containers, queryKey }: { resource: ResourceRef; task: ScheduledTask; containers?: { value: string; label: string }[]; queryKey: unknown[] }) {
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [open, setOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const executions = useQuery({
    queryKey: [...queryKey, task.id, "executions"],
    queryFn: () => get<ServerTask[]>(`/teams/${resource.team}/scheduled-tasks/${task.id}/executions`),
    enabled: open,
    refetchInterval: open ? 5000 : false,
  })
  const run = useMutation({
    mutationFn: () => post(`/teams/${resource.team}/scheduled-tasks/${task.id}/run`),
    onSuccess: () => {
      toast.success("Exécution lancée")
      setOpen(true)
      void qc.invalidateQueries({ queryKey: [...queryKey, task.id, "executions"] })
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api(`/teams/${resource.team}/scheduled-tasks/${task.id}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey }),
    onError: (e) => toast.error(e.message),
  })
  if (editing)
    return (
      <div className="border-b border-border p-5 last:border-b-0">
        <TaskForm resource={resource} task={task} containers={containers} queryKey={queryKey} onDone={() => setEditing(false)} />
      </div>
    )
  return (
    <div className="border-b border-border last:border-b-0">
      <div className="flex flex-wrap items-center gap-3 px-4 py-3">
        <button type="button" onClick={() => setOpen(!open)} className="min-w-0 flex-1 text-left">
          <p className="truncate text-sm font-semibold">
            {task.name} {!task.enabled && <span className="ml-1 text-xs font-normal text-muted-foreground">(désactivée)</span>}
          </p>
          <p className="truncate font-mono text-xs text-muted-foreground">
            {task.frequency} · {task.command}
            {task.container && ` · ${task.container}`}
          </p>
        </button>
        <span className="text-xs text-muted-foreground">Dernière exécution : {relativeTime(task.last_run_at)}</span>
        <Button size="sm" variant="outline" loading={run.isPending} onClick={() => run.mutate()}>
          <PlayIcon /> Exécuter
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setEditing(true)}>
          Modifier
        </Button>
        <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
          <Trash2Icon />
        </Button>
      </div>
      {open && (
        <div className="border-t border-border bg-foreground/[0.02]">
          {executions.data?.length ? executions.data.map((e) => <TaskRow key={e.id} task={e} title="Exécution" />) : <p className="px-5 py-4 text-xs text-muted-foreground">Aucune exécution.</p>}
        </div>
      )}
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer « ${task.name} » ?`} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </div>
  )
}
