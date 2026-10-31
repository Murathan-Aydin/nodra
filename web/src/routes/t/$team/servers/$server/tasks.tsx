import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { ListChecksIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { TaskOutput, taskLabel } from "@/components/task-output"
import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { relativeTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import { tasksQuery } from "@/features/servers/queries"

export const Route = createFileRoute("/t/$team/servers/$server/tasks")({
  validateSearch: (s: Record<string, unknown>): { task?: string } => (typeof s.task === "string" ? { task: s.task } : {}),
  component: ServerTasks,
})

function ServerTasks() {
  const { team, server } = Route.useParams()
  const { task } = Route.useSearch()
  const tasks = useQuery({ ...tasksQuery(team, server), refetchInterval: 10_000 })
  const selected = task ?? tasks.data?.[0]?.id
  if (tasks.isPending) return <Skeleton className="h-64 w-full" />
  if (!tasks.data?.length) return <EmptyState icon={ListChecksIcon} title="Aucune tâche" description="Les actions sur le serveur (validation, nettoyage…) apparaissent ici." />
  const current = tasks.data.find((t) => t.id === selected)
  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,320px)_1fr]">
      <Card className="h-fit">
        <ul className="max-h-[70vh] divide-y overflow-y-auto">
          {tasks.data.map((t) => (
            <li key={t.id}>
              <Link
                to="/t/$team/servers/$server/tasks"
                params={{ team, server }}
                search={{ task: t.id }}
                className={cn("flex items-center justify-between gap-2 px-4 py-2.5 text-sm hover:bg-muted/50", t.id === selected && "bg-accent/60")}
              >
                <span className="min-w-0">
                  <span className="block truncate font-medium">{taskLabel(t.kind)}</span>
                  <span className="text-xs text-muted-foreground">{relativeTime(t.created_at)}</span>
                </span>
                <StatusBadge status={t.status === "running" ? "in_progress" : t.status} />
              </Link>
            </li>
          ))}
        </ul>
      </Card>
      <Card>
        <CardContent className="pt-5">
          {selected ? (
            <>
              <h2 className="mb-3 font-semibold">{current ? taskLabel(current.kind) : "Tâche"}</h2>
              <TaskOutput key={selected} team={team} server={server} task={selected} />
            </>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
