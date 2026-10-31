import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { BoxIcon, ChevronDownIcon, DatabaseIcon, LayersIcon, PlusIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { get, teamPath } from "@/lib/api"
import { usePermission } from "@/lib/permissions"
import type { Environment, Project } from "@/lib/types"
import { resourcesQuery } from "@/features/projects/queries"
import { ResourceCard } from "@/features/projects/resource-card"
import { VariablesPanel } from "@/features/projects/variables-panel"

export const Route = createFileRoute("/t/$team/projects/$project/$env/")({ component: EnvironmentPage })

function EnvironmentPage() {
  const { team, project, env } = Route.useParams()
  const info = useQuery({
    queryKey: ["team", team, "project", project, "env", env],
    queryFn: () => get<{ project: Project; environment: Environment }>(teamPath(team, `/projects/${project}/environments/${env}`)),
  })
  const resources = useQuery(resourcesQuery(team, { environment_id: env }))
  const canCreate = usePermission(team, "resource.create", { type: "environment", id: env })
  const [filter, setFilter] = useState("")
  const shown = resources.data?.filter((r) => !filter || r.name.toLowerCase().includes(filter.toLowerCase()) || r.tags.some((t) => t.toLowerCase().includes(filter.toLowerCase())))
  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-2">
            <Link to="/t/$team/projects/$project" params={{ team, project }} className="text-muted-foreground hover:text-foreground">
              {info.data?.project.name ?? "…"}
            </Link>
            <span className="text-muted-foreground">/</span>
            {info.data?.environment.name ?? "…"}
          </span>
        }
        description={info.data?.environment.description}
        actions={
          canCreate && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button>
                  <PlusIcon /> Nouvelle ressource <ChevronDownIcon />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-56">
                <DropdownMenuItem asChild>
                  <Link to="/t/$team/projects/$project/$env/new" params={{ team, project, env }} search={{ type: "application" }}>
                    <BoxIcon /> Application
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link to="/t/$team/projects/$project/$env/new" params={{ team, project, env }} search={{ type: "database" }}>
                    <DatabaseIcon /> Base de données
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link to="/t/$team/projects/$project/$env/new" params={{ team, project, env }} search={{ type: "service" }}>
                    <LayersIcon /> Service one-click
                  </Link>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )
        }
      />
      <Tabs defaultValue="resources">
        <TabsList>
          <TabsTrigger value="resources">Ressources</TabsTrigger>
          <TabsTrigger value="variables">Variables de l'environnement</TabsTrigger>
        </TabsList>
        <TabsContent value="resources" className="space-y-4">
          {!!resources.data?.length && <Input className="max-w-xs" placeholder="Filtrer par nom ou tag" value={filter} onChange={(e) => setFilter(e.target.value)} />}
          {resources.isPending ? (
            <Skeleton className="h-32 w-full" />
          ) : !resources.data?.length ? (
            <EmptyState icon={BoxIcon} title="Environnement vide" description="Ajoutez une application, une base de données ou un service one-click." />
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {shown?.map((r) => (
                <ResourceCard key={r.id} team={team} resource={r} />
              ))}
            </div>
          )}
        </TabsContent>
        <TabsContent value="variables">
          <VariablesPanel team={team} scope={{ type: "environment", id: env }} />
        </TabsContent>
      </Tabs>
    </>
  )
}
