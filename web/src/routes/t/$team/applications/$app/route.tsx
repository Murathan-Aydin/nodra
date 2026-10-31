import { createFileRoute, Link, Outlet, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { ChevronDownIcon, ExternalLinkIcon, HammerIcon, RocketIcon, RotateCwIcon, SquareIcon } from "lucide-react"
import { PageHeader } from "@/components/page-header"
import { TAB_ACTIVE, TAB_LINK, TabNav } from "@/components/tab-nav"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Deployment } from "@/lib/types"
import { appBase, appQuery } from "@/features/applications/queries"

export const Route = createFileRoute("/t/$team/applications/$app")({ component: AppLayout })

const TABS = [
  { to: "/t/$team/applications/$app", label: "Général", exact: true },
  { to: "/t/$team/applications/$app/deployments", label: "Déploiements" },
  { to: "/t/$team/applications/$app/logs", label: "Logs" },
  { to: "/t/$team/applications/$app/terminal", label: "Terminal" },
  { to: "/t/$team/applications/$app/envs", label: "Variables" },
  { to: "/t/$team/applications/$app/storages", label: "Stockages" },
  { to: "/t/$team/applications/$app/previews", label: "Previews" },
  { to: "/t/$team/applications/$app/webhooks", label: "Webhooks" },
  { to: "/t/$team/applications/$app/tasks", label: "Tâches planifiées" },
  { to: "/t/$team/applications/$app/access", label: "Accès" },
  { to: "/t/$team/applications/$app/advanced", label: "Avancé" },
] as const

function AppLayout() {
  const { team, app } = Route.useParams()
  const navigate = useNavigate()
  const { data } = useQuery(appQuery(team, app))
  const canDeploy = usePermission(team, "resource.deploy", { type: "resource", id: app })
  const base = appBase(team, app)
  const action = useApiMutation((v: { path: string; body?: unknown }) => post<Deployment & { approval_request?: unknown }>(`${base}${v.path}`, v.body), {
    success: (r) => (r && "id" in r && !r.approval_request ? "Déploiement lancé" : r === undefined ? "Application arrêtée" : undefined),
    invalidate: [["team", team, "app", app]],
    onSuccess: (r) => {
      if (r && "id" in r && r.id && !r.approval_request) void navigate({ to: "/t/$team/applications/$app/deployments", params: { team, app }, search: { dep: r.id } })
    },
  })
  const a = data?.application
  return (
    <>
      {!a ? (
        <Skeleton className="mb-6 h-12 w-80" />
      ) : (
        <PageHeader
          title={
            <span className="flex items-center gap-3">
              {a.name}
              <StatusBadge status={a.status} />
            </span>
          }
          description={
            a.domains[0] ? (
              <a href={a.domains[0]} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:underline">
                {a.domains[0].replace(/^https?:\/\//, "")} <ExternalLinkIcon className="size-3.5" />
              </a>
            ) : (
              a.description || a.git_repository || a.docker_image
            )
          }
          actions={
            canDeploy && (
              <div className="flex">
                <Button className="rounded-r-none" loading={action.isPending} onClick={() => action.mutate({ path: "/deploy" })}>
                  <RocketIcon /> Déployer
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button className="rounded-l-none border-l border-primary-foreground/20 px-2" aria-label="Autres actions">
                      <ChevronDownIcon />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-60">
                    <DropdownMenuItem onSelect={() => action.mutate({ path: "/deploy", body: { force_rebuild: true } })}>
                      <HammerIcon /> Déployer sans cache
                    </DropdownMenuItem>
                    <DropdownMenuItem onSelect={() => action.mutate({ path: "/restart" })}>
                      <RotateCwIcon /> Redémarrer (sans rebuild)
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onSelect={() => action.mutate({ path: "/stop" })}>
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
          <Link
            key={t.to}
            to={t.to}
            params={{ team, app }}
            activeOptions={{ exact: "exact" in t }}
            className={TAB_LINK}
            activeProps={TAB_ACTIVE}
          >
            {t.label}
          </Link>
        ))}
      </TabNav>
      <Outlet />
    </>
  )
}
