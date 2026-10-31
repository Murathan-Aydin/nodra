import { createFileRoute, Link, Outlet } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ChevronDownIcon, PlayIcon, RotateCwIcon, SquareIcon } from "lucide-react"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { TAB_ACTIVE, TAB_LINK, TabNav } from "@/components/tab-nav"
import { TaskDialog } from "@/components/task-output"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { ServerTask } from "@/lib/types"
import { TemplateMark } from "@/features/services/create-service"
import { svcBase, svcQuery } from "@/features/services/queries"

export const Route = createFileRoute("/t/$team/services/$svc")({ component: ServiceLayout })

const TABS = [
  { to: "/t/$team/services/$svc", label: "Général", exact: true },
  { to: "/t/$team/services/$svc/logs", label: "Logs" },
  { to: "/t/$team/services/$svc/terminal", label: "Terminal" },
  { to: "/t/$team/services/$svc/envs", label: "Variables" },
  { to: "/t/$team/services/$svc/history", label: "Historique" },
  { to: "/t/$team/services/$svc/backups", label: "Sauvegardes" },
  { to: "/t/$team/services/$svc/tasks", label: "Tâches planifiées" },
  { to: "/t/$team/services/$svc/advanced", label: "Avancé" },
] as const

const RUNNING = new Set(["running", "degraded", "restarting"])

function ServiceLayout() {
  const { team, svc } = Route.useParams()
  const { data } = useQuery(svcQuery(team, svc))
  const canDeploy = usePermission(team, "resource.deploy", { type: "resource", id: svc })
  const [task, setTask] = useState<ServerTask | null>(null)
  const action = useApiMutation((path: string) => post<ServerTask>(`${svcBase(team, svc)}${path}`), {
    invalidate: [["team", team, "svc", svc]],
    onSuccess: (t) => t && "kind" in t && setTask(t),
  })
  const s = data?.service
  const running = !!s && RUNNING.has(s.status.split(":")[0])
  return (
    <>
      {!s ? (
        <Skeleton className="mb-6 h-12 w-80" />
      ) : (
        <PageHeader
          title={
            <span className="flex items-center gap-3">
              <TemplateMark name={s.name} className="size-8" />
              {s.name}
              <StatusBadge status={s.status} />
            </span>
          }
          description={s.description || (s.template ? `Modèle ${s.template}` : "Compose personnalisé")}
          actions={
            canDeploy && (
              <div className="flex">
                {running ? (
                  <Button className="rounded-r-none" variant="outline" loading={action.isPending} onClick={() => action.mutate("/restart")}>
                    <RotateCwIcon /> Redémarrer
                  </Button>
                ) : (
                  <Button className="rounded-r-none" loading={action.isPending} onClick={() => action.mutate("/start")}>
                    <PlayIcon /> Démarrer
                  </Button>
                )}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant={running ? "outline" : "default"} className="rounded-l-none border-l px-2" aria-label="Autres actions">
                      <ChevronDownIcon />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-56">
                    <DropdownMenuItem onSelect={() => action.mutate("/start")}>
                      <PlayIcon /> Démarrer (recréer)
                    </DropdownMenuItem>
                    <DropdownMenuItem onSelect={() => action.mutate("/restart")}>
                      <RotateCwIcon /> Redémarrer
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onSelect={() => action.mutate("/stop")}>
                      <SquareIcon /> Arrêter
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            )
          }
        />
      )}
      <TabNav>
        {TABS.map((t) => (
          <Link key={t.to} to={t.to} params={{ team, svc }} activeOptions={{ exact: "exact" in t }} className={TAB_LINK} activeProps={TAB_ACTIVE}>
            {t.label}
          </Link>
        ))}
      </TabNav>
      <Outlet />
      <TaskDialog team={team} task={task} onOpenChange={(o) => !o && setTask(null)} />
    </>
  )
}
