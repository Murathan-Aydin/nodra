import { useQuery } from "@tanstack/react-query"
import { DatabaseIcon, ExternalLinkIcon, GlobeIcon, LayoutGridIcon } from "lucide-react"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { serviceQueries, templateQuery } from "@/lib/queries"
import { resourceStatus } from "@/lib/status"
import type { Container, Service, ServiceApp } from "@/lib/types"
import { cn } from "@/lib/utils"

const COLS = "grid-cols-[minmax(14rem,1fr)_minmax(12rem,1fr)_12rem_5rem]"

// Images de bases de données : icône base dans la liste (comme Coolify).
const DATABASE_IMAGE = /(^|\/)(mariadb|mysql|postgres|postgis|pgvector|mongo|redis|keydb|dragonfly|valkey|clickhouse)[:@]?/

/** espocrm-daemon → « Espocrm Daemon ». */
export const displayName = (name: string) => name.split(/[-_]/).map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(" ")

/** Statut d'un conteneur au format de resourceStatus (« running:healthy », « exited »…). */
function containerStatus(c: Container | undefined): string {
  if (!c) return "stopped"
  if (c.state === "running") return c.status.includes("unhealthy") ? "running:unhealthy" : "running:healthy"
  return c.state
}

/** « Ressources du compose » : un conteneur par service du docker-compose, avec son image et son statut. */
export function ComposeResources({ team, service, apps }: { team: string; service: Service; apps: ServiceApp[] }) {
  const containers = useQuery(serviceQueries.containers(team, service.id)).data ?? []
  const doc = useQuery(templateQuery(service.template)).data?.documentation
  return (
    <SettingsCard
      title="Ressources du compose"
      bodyClassName="overflow-x-auto"
      actions={
        doc && (
          <Button size="sm" variant="outline" asChild>
            <a href={doc} target="_blank" rel="noreferrer">
              Documentation <ExternalLinkIcon />
            </a>
          </Button>
        )
      }
    >
      <div className={cn("grid min-w-[48rem] gap-3 border-b border-border px-4 py-2.5 text-[11px] font-medium text-muted-foreground", COLS)}>
        <div>Ressource</div>
        <div>Image</div>
        <div>Statut</div>
        <div />
      </div>
      {apps.map((app) => {
        const status = resourceStatus(containerStatus(containers.find((x) => x.service === app.name)))
        const domain = app.domains?.[0]
        const Icon = DATABASE_IMAGE.test(app.image) ? DatabaseIcon : LayoutGridIcon
        return (
          <div key={app.id} className={cn("grid min-w-[48rem] items-center gap-3 border-b border-border px-4 py-3 last:border-b-0", COLS)}>
            <div className="flex min-w-0 items-center gap-3">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-muted-foreground">
                <Icon className="size-4" />
              </span>
              <span className="truncate text-sm font-semibold" title={app.name}>
                {displayName(app.name)}
              </span>
            </div>
            <div className="truncate font-mono text-xs text-muted-foreground">{app.image}</div>
            <div>
              <span className="inline-flex h-[22px] items-center gap-1.5 rounded-full border border-border bg-muted px-2.5 text-xs font-medium">
                <span className={cn("size-1.5 rounded-full", status.dot)} />
                {status.label}
              </span>
            </div>
            <div className="flex justify-end">
              {domain && (
                <a
                  href={/^https?:\/\//.test(domain) ? domain : `https://${domain}`}
                  target="_blank"
                  rel="noreferrer"
                  title={domain}
                  className="flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-foreground/5 hover:text-foreground"
                >
                  <GlobeIcon className="size-4" />
                </a>
              )}
            </div>
          </div>
        )
      })}
      {apps.length === 0 && <p className="px-4 py-6 text-center text-sm text-muted-foreground">Aucun conteneur dans ce compose.</p>}
    </SettingsCard>
  )
}
