import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ChevronRightIcon } from "lucide-react"
import { SettingsCard } from "@/components/page"
import { TaskStatus } from "@/components/task-status"
import { Skeleton } from "@/components/ui/skeleton"
import { get } from "@/lib/api"
import { dateTime, duration, relativeTime } from "@/lib/format"
import type { ServerTask } from "@/lib/types"
import { cn } from "@/lib/utils"
import { resourceApi, resourceKey, type ResourceRef } from "./resource"

const KIND: Record<string, string> = {
  service_start: "Démarrage",
  service_stop: "Arrêt",
  service_restart: "Redémarrage",
  service_restart_app: "Redémarrage d'un conteneur",
  database_start: "Démarrage",
  database_stop: "Arrêt",
  database_restart: "Redémarrage",
  database_proxy: "Accès public",
  scheduled_task: "Tâche planifiée",
}

export const taskLabel = (kind: string) => KIND[kind] ?? kind.replaceAll("_", " ")

type Task = ServerTask & { finished_at?: string | null; started_at?: string | null }

/** Onglet « Historique » : actions serveur lancées sur la ressource (démarrages, arrêts…) avec leur sortie. */
export function HistoryTab({ resource }: { resource: ResourceRef }) {
  const tasks = useQuery({ queryKey: resourceKey(resource, "tasks"), queryFn: () => get<Task[]>(resourceApi(resource, "/tasks")), refetchInterval: 10_000 })
  return (
    <SettingsCard title="Historique" description="50 dernières actions sur la ressource." bodyClassName="p-0">
      {tasks.isPending ? (
        <Skeleton className="m-4 h-24" />
      ) : tasks.data?.length ? (
        tasks.data.map((t) => <TaskRow key={t.id} task={t} />)
      ) : (
        <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune action pour l'instant.</p>
      )}
    </SettingsCard>
  )
}

export function TaskRow({ task, title }: { task: Task; title?: string }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="border-b border-border last:border-b-0">
      <button type="button" onClick={() => setOpen(!open)} className="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-foreground/[0.03]">
        <ChevronRightIcon className={cn("size-4 shrink-0 text-muted-foreground transition-transform", open && "rotate-90")} />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{title ?? taskLabel(task.kind)}</span>
        <span className="hidden text-xs text-muted-foreground sm:inline" title={dateTime(task.created_at)}>
          {relativeTime(task.created_at)}
        </span>
        <span className="hidden w-24 text-right text-xs text-muted-foreground md:inline">{task.started_at ? duration(task.started_at, task.finished_at) : ""}</span>
        <TaskStatus status={task.status} />
      </button>
      {open && (
        <pre className="max-h-96 overflow-auto border-t border-border bg-black/30 px-4 py-3 font-mono text-xs leading-5 whitespace-pre-wrap">
          {task.output || task.error || "Pas de sortie."}
          {task.output && task.error && `\n\n${task.error}`}
        </pre>
      )}
    </div>
  )
}
