import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { LayersIcon, PlusIcon, SettingsIcon } from "lucide-react"
import { CardLink, PageHeader } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { projectQueries, teamQueries } from "@/lib/queries"
import { CreateEnvironmentDialog } from "@/features/projects/dialogs"

export const Route = createFileRoute("/t/$team/projects/$project/")({ component: Project })

function Project() {
  const { team, project } = Route.useParams()
  const data = useQuery(projectQueries.project(team, project)).data
  const resources = useQuery(teamQueries.resources(team)).data ?? []
  const [creating, setCreating] = useState(false)
  return (
    <>
      <PageHeader
        crumbs={[{ label: "Projets", to: `/t/${team}/projects` }, { label: data?.project.name ?? "…" }]}
        title={data?.project.name ?? <Skeleton className="h-8 w-48" />}
        description={data?.project.description || "Environnements du projet."}
        actions={
          <>
            <Button size="sm" variant="outline" onClick={() => setCreating(true)}>
              <PlusIcon /> Environnement
            </Button>
            <Button size="sm" variant="ghost" asChild>
              <Link to="/t/$team/projects/$project/settings" params={{ team, project }}>
                <SettingsIcon /> Réglages
              </Link>
            </Button>
          </>
        }
      />
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {data?.environments.map((e) => {
          const count = resources.filter((r) => r.environment_id === e.id).length
          return (
            <CardLink key={e.id} to={`/t/${team}/projects/${project}/${e.id}`}>
              <div className="flex items-start gap-3">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-border bg-muted">
                  <LayersIcon className="size-5" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium group-hover:text-primary">{e.name}</span>
                  <span className="block truncate text-sm text-muted-foreground">{e.description || `${count} ressource${count > 1 ? "s" : ""}`}</span>
                </span>
              </div>
            </CardLink>
          )
        })}
      </div>
      <CreateEnvironmentDialog team={team} project={project} open={creating} onOpenChange={setCreating} />
    </>
  )
}
