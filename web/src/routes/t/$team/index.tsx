import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { BoxIcon, DatabaseIcon, LayersIcon, RocketIcon, ServerIcon, ShieldCheckIcon } from "lucide-react"
import type { ReactNode } from "react"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { get, teamPath } from "@/lib/api"
import { relativeTime } from "@/lib/format"
import { teamQuery } from "@/lib/queries"
import type { ApprovalRequest, Deployment, ResourceSummary, ServerSummary } from "@/lib/types"

export const Route = createFileRoute("/t/$team/")({ component: Dashboard })

const TYPE_ICON = { application: BoxIcon, database: DatabaseIcon, service: LayersIcon }
const TYPE_LABEL = { application: "Application", database: "Base de données", service: "Service" }

function Dashboard() {
  const { team } = Route.useParams()
  const access = useQuery(teamQuery(team))
  const resources = useQuery({ queryKey: ["team", team, "resources"], queryFn: () => get<ResourceSummary[]>(teamPath(team, "/resources")) })
  const servers = useQuery({ queryKey: ["team", team, "servers"], queryFn: () => get<ServerSummary[]>(teamPath(team, "/servers")) })
  const deployments = useQuery({
    queryKey: ["team", team, "deployments", { limit: 8 }],
    queryFn: () => get<Deployment[]>(teamPath(team, "/deployments"), { limit: 8 }),
  })
  const approvals = useQuery({
    queryKey: ["team", team, "approvals", "pending"],
    queryFn: () => get<ApprovalRequest[]>(teamPath(team, "/approvals"), { status: "pending" }),
  })

  const running = resources.data?.filter((r) => r.status === "running").length ?? 0
  const names = new Map(resources.data?.map((r) => [r.id, r.name]))

  return (
    <>
      <PageHeader title={access.data?.team.name ?? "Tableau de bord"} description="Vue d'ensemble de vos ressources, serveurs et déploiements." />
      <div className="mb-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Ressources" value={resources.data?.length} hint={resources.data ? `${running} en cours d'exécution` : undefined} icon={BoxIcon} />
        <Stat
          label="Serveurs"
          value={servers.data?.length}
          hint={servers.data ? `${servers.data.filter((s) => s.is_reachable).length} joignables` : undefined}
          icon={ServerIcon}
        />
        <Stat
          label="Déploiements récents"
          value={deployments.data?.length}
          hint={deployments.data ? `${deployments.data.filter((d) => d.status === "failed").length} en échec` : undefined}
          icon={RocketIcon}
        />
        <Stat label="Approbations en attente" value={approvals.data?.length} icon={ShieldCheckIcon} />
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Ressources</CardTitle>
            <Button variant="outline" size="sm" asChild>
              <Link to="/t/$team/projects" params={{ team }}>
                Projets
              </Link>
            </Button>
          </CardHeader>
          <CardContent>
            {resources.isPending ? (
              <ListSkeleton />
            ) : !resources.data?.length ? (
              <EmptyState icon={BoxIcon} title="Aucune ressource" description="Créez un projet puis déployez une application, une base de données ou un service." />
            ) : (
              <ul className="divide-y">
                {resources.data.slice(0, 10).map((r) => {
                  const Icon = TYPE_ICON[r.type]
                  return (
                    <li key={r.id} className="flex items-center gap-3 py-2.5">
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-accent text-accent-foreground">
                        <Icon className="size-4" />
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium">{r.name}</p>
                        <p className="truncate text-xs text-muted-foreground">
                          {TYPE_LABEL[r.type]} · {r.project_name} / {r.environment_name}
                          {r.domains[0] && <> · {r.domains[0].replace(/^https?:\/\//, "")}</>}
                        </p>
                      </div>
                      <StatusBadge status={r.status} />
                    </li>
                  )
                })}
              </ul>
            )}
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Serveurs</CardTitle>
            </CardHeader>
            <CardContent>
              {servers.isPending ? (
                <ListSkeleton rows={2} />
              ) : !servers.data?.length ? (
                <EmptyState
                  icon={ServerIcon}
                  title="Aucun serveur"
                  action={
                    <Button size="sm" asChild>
                      <Link to="/t/$team/servers" params={{ team }}>
                        Ajouter un serveur
                      </Link>
                    </Button>
                  }
                />
              ) : (
                <ul className="space-y-2.5">
                  {servers.data.map((s) => (
                    <li key={s.id} className="flex items-center justify-between gap-2 text-sm">
                      <span className="min-w-0">
                        <span className="block truncate font-medium">{s.name}</span>
                        <span className="block truncate text-xs text-muted-foreground">{s.ip}</span>
                      </span>
                      <StatusBadge status={s.is_reachable ? "reachable" : "unreachable"} />
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Derniers déploiements</CardTitle>
            </CardHeader>
            <CardContent>
              {deployments.isPending ? (
                <ListSkeleton rows={3} />
              ) : !deployments.data?.length ? (
                <p className="text-sm text-muted-foreground">Aucun déploiement pour l'instant.</p>
              ) : (
                <ul className="space-y-2.5">
                  {deployments.data.map((d) => (
                    <li key={d.id} className="flex items-center justify-between gap-2 text-sm">
                      <span className="min-w-0">
                        <span className="block truncate font-medium">{names.get(d.application_id) ?? "Application"}</span>
                        <span className="block truncate text-xs text-muted-foreground">
                          {relativeTime(d.created_at)} · {d.commit_sha ? d.commit_sha.slice(0, 7) : d.trigger}
                          {d.pr_number > 0 && ` · PR #${d.pr_number}`}
                        </span>
                      </span>
                      <StatusBadge status={d.status} />
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </>
  )
}

function Stat({ label, value, hint, icon: Icon }: { label: string; value?: number; hint?: ReactNode; icon: typeof BoxIcon }) {
  return (
    <Card>
      <CardContent className="flex items-start justify-between pt-5">
        <div>
          <p className="text-sm text-muted-foreground">{label}</p>
          {value === undefined ? <Skeleton className="mt-2 h-7 w-12" /> : <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>}
          {hint && <p className="mt-1 text-xs text-muted-foreground">{hint}</p>}
        </div>
        <Icon className="size-5 text-muted-foreground" />
      </CardContent>
    </Card>
  )
}

function ListSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-9 w-full" />
      ))}
    </div>
  )
}
