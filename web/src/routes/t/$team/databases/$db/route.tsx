import { createFileRoute, Link, Outlet } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ChevronDownIcon, PlayIcon, RotateCwIcon, SquareIcon } from "lucide-react"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { TAB_ACTIVE, TAB_LINK, TabNav } from "@/components/tab-nav"
import { TaskDialog } from "@/components/task-output"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { ServerTask } from "@/lib/types"
import { DB_TYPES, dbBase, dbQuery } from "@/features/databases/queries"

export const Route = createFileRoute("/t/$team/databases/$db")({ component: DatabaseLayout })

const TABS = [
  { to: "/t/$team/databases/$db", label: "Général", exact: true },
  { to: "/t/$team/databases/$db/backups", label: "Sauvegardes" },
  { to: "/t/$team/databases/$db/logs", label: "Logs" },
  { to: "/t/$team/databases/$db/terminal", label: "Terminal" },
  { to: "/t/$team/databases/$db/history", label: "Historique" },
  { to: "/t/$team/databases/$db/envs", label: "Variables" },
  { to: "/t/$team/databases/$db/storages", label: "Stockages" },
  { to: "/t/$team/databases/$db/tasks", label: "Tâches planifiées" },
  { to: "/t/$team/databases/$db/advanced", label: "Avancé" },
] as const

const RUNNING = new Set(["running", "degraded", "restarting"])

function DatabaseLayout() {
  const { team, db } = Route.useParams()
  const { data } = useQuery(dbQuery(team, db))
  const canDeploy = usePermission(team, "resource.deploy", { type: "resource", id: db })
  const [task, setTask] = useState<ServerTask | null>(null)
  const action = useApiMutation((path: string) => post<ServerTask>(`${dbBase(team, db)}${path}`), {
    invalidate: [["team", team, "db", db]],
    onSuccess: (t) => t && "kind" in t && setTask(t),
  })
  const d = data?.database
  const running = !!d && RUNNING.has(d.status.split(":")[0])
  return (
    <>
      {!d ? (
        <Skeleton className="mb-6 h-12 w-80" />
      ) : (
        <PageHeader
          title={
            <span className="flex items-center gap-3">
              {d.name}
              <StatusBadge status={d.status} />
              {d.is_public && <Badge variant="outline">Publique</Badge>}
            </span>
          }
          description={`${DB_TYPES[d.type].label} · ${d.image}`}
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
          <Link key={t.to} to={t.to} params={{ team, db }} activeOptions={{ exact: "exact" in t }} className={TAB_LINK} activeProps={TAB_ACTIVE}>
            {t.label}
          </Link>
        ))}
      </TabNav>
      <Outlet />
      <TaskDialog team={team} task={task} onOpenChange={(o) => !o && setTask(null)} />
    </>
  )
}
