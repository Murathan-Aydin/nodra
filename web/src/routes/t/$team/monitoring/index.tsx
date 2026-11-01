import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { ActivityIcon } from "lucide-react"
import { EmptyPanel, PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { DEPLOY_STATUS } from "@/features/applications/deployments"
import { resourcePage } from "@/features/resources/resource"
import { ServerMetrics } from "@/features/servers/tools"
import { relativeTime } from "@/lib/format"
import { teamQueries } from "@/lib/queries"
import { resourceStatus } from "@/lib/status"

export const Route = createFileRoute("/t/$team/monitoring/")({ component: Monitoring })

/** Supervision (Monitoring du fork) : métriques du serveur, état des ressources, déploiements récents. */
function Monitoring() {
  const { team } = Route.useParams()
  const server = useQuery(teamQueries.servers(team)).data?.[0]
  const resources = useQuery(teamQueries.resources(team)).data ?? []
  const deployments = useQuery(teamQueries.deployments(team)).data ?? []
  const up = resources.filter((r) => r.status.startsWith("running")).length
  return (
    <>
      <PageHeader title="Supervision" description={`${up} ressource(s) en cours sur ${resources.length}.`} />
      <div className="space-y-6">
        {server ? <ServerMetrics team={team} server={server.id} /> : <EmptyPanel icon={<ActivityIcon />} title="Aucun serveur" />}
        <div className="grid gap-6 lg:grid-cols-2">
          <SettingsCard title="Ressources" bodyClassName="p-0">
            {resources.map((r) => {
              const s = resourceStatus(r.status)
              return (
                <Link key={r.id} to={resourcePage({ team, kind: r.type, id: r.id })} className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0 hover:bg-foreground/[0.03]">
                  <span className="min-w-0 flex-1 truncate text-sm">{r.name}</span>
                  <span className="hidden truncate text-xs text-muted-foreground sm:inline">
                    {r.project_name} / {r.environment_name}
                  </span>
                  <Pill label={s.label} dot={s.dot} />
                </Link>
              )
            })}
          </SettingsCard>
          <SettingsCard title="Déploiements récents" bodyClassName="p-0">
            {deployments.map((d) => {
              const s = DEPLOY_STATUS[d.status]
              return (
                <Link key={d.id} to="/t/$team/applications/$application/deployments/$deployment" params={{ team, application: d.application_id, deployment: d.id }} className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0 hover:bg-foreground/[0.03]">
                  <span className="min-w-0 flex-1 truncate text-sm">{resources.find((r) => r.id === d.application_id)?.name ?? "Application"}</span>
                  <span className="text-xs text-muted-foreground">{relativeTime(d.created_at)}</span>
                  <Pill label={s.label} dot={s.dot} />
                </Link>
              )
            })}
            {deployments.length === 0 && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucun déploiement.</p>}
          </SettingsCard>
        </div>
      </div>
    </>
  )
}
