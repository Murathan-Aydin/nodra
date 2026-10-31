import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { BoxIcon, CopyIcon, DatabaseIcon, LayersIcon, PlusIcon, SearchIcon, SettingsIcon } from "lucide-react"
import { EmptyPanel, PageHeader } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { projectQueries, teamQueries } from "@/lib/queries"
import { resourceStatus } from "@/lib/status"
import type { ResourceSummary } from "@/lib/types"
import { cn } from "@/lib/utils"
import { CloneEnvironmentDialog } from "@/features/projects/clone-environment"

export const Route = createFileRoute("/t/$team/projects/$project/$env/")({ component: EnvironmentPage })

const TYPE: Record<ResourceSummary["type"], { label: string; icon: typeof BoxIcon; path: string }> = {
  application: { label: "Application", icon: BoxIcon, path: "applications" },
  database: { label: "Base de données", icon: DatabaseIcon, path: "databases" },
  service: { label: "Service", icon: LayersIcon, path: "services" },
}

const COLS = "grid-cols-[minmax(0,1.4fr)_9rem_9rem_minmax(0,1.2fr)_8rem]"

function EnvironmentPage() {
  const { team, project, env } = Route.useParams()
  const data = useQuery(projectQueries.environment(team, project, env)).data
  const resources = useQuery(projectQueries.resources(team, env))
  const servers = useQuery(teamQueries.servers(team)).data ?? []
  const [q, setQ] = useState("")
  const [cloning, setCloning] = useState(false)
  const list = (resources.data ?? []).filter((r) => !q || r.name.toLowerCase().includes(q.toLowerCase()))
  const newPath = { to: "/t/$team/projects/$project/$env/new", params: { team, project, env } } as const

  return (
    <>
      <PageHeader
        crumbs={[
          { label: "Projets", to: `/t/${team}/projects` },
          { label: data?.project.name ?? "…", to: `/t/${team}/projects/${project}` },
          { label: data?.environment.name ?? "…" },
        ]}
        title={data?.environment.name ?? <Skeleton className="h-8 w-40" />}
        description={data?.environment.description || "Ressources de cet environnement."}
        actions={
          <>
            <Button size="sm" asChild>
              <Link {...newPath}>
                <PlusIcon /> Nouvelle ressource
              </Link>
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setCloning(true)}>
              <CopyIcon /> Cloner
            </Button>
            <Button size="sm" variant="ghost" asChild>
              <Link to="/t/$team/projects/$project/$env/settings" params={{ team, project, env }}>
                <SettingsIcon /> Réglages
              </Link>
            </Button>
          </>
        }
      />
      {resources.isPending ? (
        <Skeleton className="h-40 rounded-2xl" />
      ) : !resources.data?.length ? (
        <EmptyPanel
          icon={<BoxIcon />}
          title="Aucune ressource pour l'instant"
          text="Déployez une application depuis Git ou une image, une base de données ou un service prêt à l'emploi."
          action={
            <Button asChild>
              <Link {...newPath}>
                <PlusIcon /> Nouvelle ressource
              </Link>
            </Button>
          }
        />
      ) : (
        <>
          <div className="relative mb-3 max-w-sm">
            <SearchIcon className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input className="pl-9" placeholder="Rechercher une ressource" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Rechercher une ressource" />
          </div>
          <div className="glass-panel overflow-hidden rounded-2xl">
            <div className={cn("grid gap-4 border-b border-border px-5 py-3 text-xs font-medium text-muted-foreground", COLS)}>
              <span>Ressource</span>
              <span>Type</span>
              <span>Statut</span>
              <span>Domaine</span>
              <span>Serveur</span>
            </div>
            <div className="divide-y divide-border">
              {list.map((r) => {
                const t = TYPE[r.type]
                const st = resourceStatus(r.status)
                const domain = r.domains?.[0]
                return (
                  <div key={r.id} className={cn("relative grid items-center gap-4 px-5 py-3.5 text-sm transition-colors hover:bg-sidebar-accent/50", COLS)}>
                    <Link to={`/t/${team}/${t.path}/${r.id}` as "/"} className="flex min-w-0 items-center gap-3 font-medium after:absolute after:inset-0">
                      <span className="flex size-9 shrink-0 items-center justify-center rounded-xl border border-primary/25 bg-primary/10 text-primary">
                        <t.icon className="size-4" />
                      </span>
                      <span className="truncate">{r.name}</span>
                    </Link>
                    <span className="text-muted-foreground">{t.label}</span>
                    <span className="flex items-center gap-2">
                      <span className={cn("size-2 rounded-full", st.dot)} />
                      {st.label}
                    </span>
                    <span className="truncate">
                      {domain ? (
                        <a href={domain} target="_blank" rel="noreferrer" className="relative z-10 text-primary hover:underline">
                          {domain.replace(/^https?:\/\//, "")}
                        </a>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </span>
                    <span className="truncate text-muted-foreground">{servers.find((s) => s.id === r.server_id)?.name ?? "—"}</span>
                  </div>
                )
              })}
              {!list.length && <p className="px-5 py-6 text-center text-sm text-muted-foreground">Aucune ressource ne correspond.</p>}
            </div>
          </div>
        </>
      )}
      {data && <CloneEnvironmentDialog team={team} project={project} environment={data.environment} open={cloning} onOpenChange={setCloning} />}
    </>
  )
}
