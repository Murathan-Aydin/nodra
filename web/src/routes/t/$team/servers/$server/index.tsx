import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { AlertTriangleIcon, GlobeIcon } from "lucide-react"
import type { ReactNode } from "react"
import { StatusBadge } from "@/components/status-badge"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { get, teamPath } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { MetricsCharts } from "@/features/servers/metrics-chart"
import { serverQuery, serverResourcesQuery } from "@/features/servers/queries"

export const Route = createFileRoute("/t/$team/servers/$server/")({ component: ServerOverview })

const TYPE_LABEL = { application: "Application", database: "Base de données", service: "Service" }

function ServerOverview() {
  const { team, server } = Route.useParams()
  const { data } = useQuery(serverQuery(team, server))
  const resources = useQuery(serverResourcesQuery(team, server))
  const domains = useQuery({
    queryKey: ["team", team, "server", server, "domains"],
    queryFn: () => get<{ domain: string; resource_id: string; type: string; name: string }[]>(teamPath(team, `/servers/${server}/domains`)),
  })
  const sv = data?.server
  if (!sv) return null
  return (
    <div className="space-y-6">
      {sv.validation_error && (
        <Alert variant="destructive">
          <AlertTriangleIcon />
          <AlertTitle>Validation en échec</AlertTitle>
          <AlertDescription>{sv.validation_error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardContent className="grid gap-x-8 gap-y-4 pt-5 sm:grid-cols-2 lg:grid-cols-4">
          <Info label="Système">{sv.os_id ? `${sv.os_id} ${sv.os_version}` : "—"}</Info>
          <Info label="Docker">{sv.docker_version || "—"}</Info>
          <Info label="Docker Compose">{sv.compose_version || "—"}</Info>
          <Info label="Proxy">
            {sv.proxy.type === "none" ? (
              "Aucun"
            ) : (
              <span className="flex items-center gap-2 capitalize">
                {sv.proxy.type} <StatusBadge status={sv.proxy.status} />
              </span>
            )}
          </Info>
          <Info label="Dernière activité">{relativeTime(sv.last_seen_at)}</Info>
          <Info label="Validé le">{dateTime(sv.validated_at)}</Info>
          <Info label="Dernier nettoyage">{relativeTime(sv.last_cleanup_at)}</Info>
          <Info label="Destinations">{data.destinations.map((d) => d.network).join(", ") || "—"}</Info>
        </CardContent>
      </Card>
      {sv.settings.metrics_enabled && <MetricsCharts team={team} server={server} />}
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Ressources hébergées</CardTitle>
          </CardHeader>
          <CardContent>
            {!resources.data?.length ? (
              <p className="text-sm text-muted-foreground">Aucune ressource sur ce serveur.</p>
            ) : (
              <ul className="divide-y">
                {resources.data.map((r) => (
                  <li key={r.id} className="flex items-center justify-between gap-2 py-2 text-sm">
                    <span className="min-w-0">
                      <span className="block truncate font-medium">{r.name}</span>
                      <span className="block text-xs text-muted-foreground">
                        {TYPE_LABEL[r.type]} · {r.project_name} / {r.environment_name}
                      </span>
                    </span>
                    <StatusBadge status={r.status} />
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Domaines servis</CardTitle>
          </CardHeader>
          <CardContent>
            {!domains.data?.length ? (
              <p className="text-sm text-muted-foreground">Aucun domaine.</p>
            ) : (
              <ul className="space-y-2">
                {domains.data.map((d) => (
                  <li key={d.domain + d.resource_id} className="flex items-center gap-2 text-sm">
                    <GlobeIcon className="size-4 text-muted-foreground" />
                    <a href={d.domain} target="_blank" rel="noreferrer" className="truncate hover:underline">
                      {d.domain.replace(/^https?:\/\//, "")}
                    </a>
                    <span className="ml-auto shrink-0 text-xs text-muted-foreground">{d.name}</span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function Info({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="mt-0.5 truncate text-sm font-medium">{children}</div>
    </div>
  )
}
