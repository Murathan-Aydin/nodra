import { Link } from "@tanstack/react-router"
import { BoxIcon, DatabaseIcon, GlobeIcon, LayersIcon } from "lucide-react"
import { StatusBadge } from "@/components/status-badge"
import { Card } from "@/components/ui/card"
import type { ResourceSummary } from "@/lib/types"
import { TagChip } from "./tags-panel"

const ICON = { application: BoxIcon, database: DatabaseIcon, service: LayersIcon }
const LABEL = { application: "Application", database: "Base de données", service: "Service" }

/** Lien vers la page d'une ressource selon son type. */
export function ResourceLink({ team, resource, children, className }: { team: string; resource: Pick<ResourceSummary, "id" | "type">; children: React.ReactNode; className?: string }) {
  if (resource.type === "application")
    return (
      <Link to="/t/$team/applications/$app" params={{ team, app: resource.id }} className={className}>
        {children}
      </Link>
    )
  if (resource.type === "database")
    return (
      <Link to="/t/$team/databases/$db" params={{ team, db: resource.id }} className={className}>
        {children}
      </Link>
    )
  return (
    <Link to="/t/$team/services/$svc" params={{ team, svc: resource.id }} className={className}>
      {children}
    </Link>
  )
}

export function ResourceCard({ team, resource: r }: { team: string; resource: ResourceSummary }) {
  const Icon = ICON[r.type]
  return (
    <ResourceLink team={team} resource={r} className="block">
      <Card className="h-full p-4 transition-colors hover:border-primary/40 hover:bg-accent/30">
        <div className="flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent text-accent-foreground">
            <Icon className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate font-medium">{r.name}</p>
            <p className="text-xs text-muted-foreground">{LABEL[r.type]}</p>
          </div>
          <StatusBadge status={r.status} />
        </div>
        {r.domains.length > 0 && (
          <p className="mt-3 flex items-center gap-1.5 truncate text-xs text-muted-foreground">
            <GlobeIcon className="size-3.5 shrink-0" />
            {r.domains[0].replace(/^https?:\/\//, "")}
            {r.domains.length > 1 && ` +${r.domains.length - 1}`}
          </p>
        )}
        {r.tags.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-1">
            {r.tags.map((t) => (
              <TagChip key={t} name={t} />
            ))}
          </div>
        )}
      </Card>
    </ResourceLink>
  )
}
