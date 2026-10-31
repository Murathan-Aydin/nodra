import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { FolderKanbanIcon, LayersIcon, LayoutGridIcon, PlusIcon } from "lucide-react"
import { CardLink, EmptyPanel, PageHeader } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { teamQueries } from "@/lib/queries"
import { CreateProjectDialog } from "@/features/projects/dialogs"

export const Route = createFileRoute("/t/$team/projects/")({ component: Projects })

function Projects() {
  const { team } = Route.useParams()
  const projects = useQuery(teamQueries.projects(team))
  const resources = useQuery(teamQueries.resources(team))
  const [creating, setCreating] = useState(false)
  return (
    <>
      <PageHeader
        title="Projets"
        description="Tous vos projets. Chacun regroupe des environnements, qui contiennent vos ressources."
        actions={
          <Button size="sm" variant="outline" onClick={() => setCreating(true)}>
            <PlusIcon /> Ajouter
          </Button>
        }
      />
      {projects.isPending ? (
        <Skeleton className="h-32 rounded-2xl" />
      ) : !projects.data?.length ? (
        <EmptyPanel icon={<FolderKanbanIcon />} title="Aucun projet" text="Créez un projet pour y déployer applications, bases de données et services." action={<Button onClick={() => setCreating(true)}>Créer un projet</Button>} />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {projects.data.map((p) => (
            <CardLink key={p.id} to={`/t/${team}/projects/${p.id}`}>
              <div className="flex items-start gap-3">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-primary/25 bg-primary/10 text-primary">
                  <LayoutGridIcon className="size-5" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium group-hover:text-primary">{p.name}</span>
                  <span className="block truncate text-sm text-muted-foreground">{p.description || "Aucune description"}</span>
                </span>
              </div>
              <div className="mt-4 flex gap-4 text-xs text-muted-foreground">
                <span className="flex items-center gap-1.5">
                  <LayersIcon className="size-3.5" /> {p.environment_count} environnement{p.environment_count > 1 ? "s" : ""}
                </span>
                <span className="flex items-center gap-1.5">
                  <LayoutGridIcon className="size-3.5" /> {resources.data?.filter((r) => r.project_id === p.id).length ?? 0} ressource(s)
                </span>
              </div>
            </CardLink>
          ))}
        </div>
      )}
      <CreateProjectDialog team={team} open={creating} onOpenChange={setCreating} />
    </>
  )
}
