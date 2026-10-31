import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { FolderKanbanIcon, PlusIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { post, teamPath } from "@/lib/api"
import { relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Project } from "@/lib/types"
import { projectsQuery, resourcesQuery } from "@/features/projects/queries"
import { TagsPanel } from "@/features/projects/tags-panel"
import { VariablesPanel } from "@/features/projects/variables-panel"

export const Route = createFileRoute("/t/$team/projects/")({ component: ProjectsPage })

function ProjectsPage() {
  const { team } = Route.useParams()
  const navigate = useNavigate()
  const projects = useQuery(projectsQuery(team))
  const resources = useQuery(resourcesQuery(team))
  const canCreate = usePermission(team, "project.create")
  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState({ name: "", description: "" })
  const create = useApiMutation((v: typeof form) => post<Project>(teamPath(team, "/projects"), v), {
    success: "Projet créé (environnement production inclus)",
    invalidate: [projectsQuery(team).queryKey],
    onSuccess: (p) => {
      setCreating(false)
      setForm({ name: "", description: "" })
      void navigate({ to: "/t/$team/projects/$project", params: { team, project: p.id } })
    },
  })
  const count = (p: string) => resources.data?.filter((r) => r.project_id === p).length ?? 0
  return (
    <>
      <PageHeader
        title="Projets"
        description="Regroupent vos ressources par environnement (production, staging…)."
        actions={
          canCreate && (
            <Button onClick={() => setCreating(true)}>
              <PlusIcon /> Nouveau projet
            </Button>
          )
        }
      />
      <Tabs defaultValue="projects">
        <TabsList>
          <TabsTrigger value="projects">Projets</TabsTrigger>
          <TabsTrigger value="variables">Variables d'équipe</TabsTrigger>
          <TabsTrigger value="tags">Tags</TabsTrigger>
        </TabsList>
        <TabsContent value="projects">
          {projects.isPending ? (
            <Skeleton className="h-32 w-full" />
          ) : !projects.data?.length ? (
            <EmptyState
              icon={FolderKanbanIcon}
              title="Aucun projet"
              description="Un projet contient des environnements, eux-mêmes composés d'applications, de bases et de services."
              action={canCreate && <Button onClick={() => setCreating(true)}>Créer un projet</Button>}
            />
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {projects.data.map((p) => (
                <Link key={p.id} to="/t/$team/projects/$project" params={{ team, project: p.id }}>
                  <Card className="h-full p-5 transition-colors hover:border-primary/40 hover:bg-accent/30">
                    <div className="flex items-start gap-3">
                      <span className="flex size-10 items-center justify-center rounded-lg bg-accent text-accent-foreground">
                        <FolderKanbanIcon className="size-5" />
                      </span>
                      <div className="min-w-0">
                        <p className="truncate font-semibold">{p.name}</p>
                        <p className="line-clamp-2 text-sm text-muted-foreground">{p.description || "Sans description"}</p>
                      </div>
                    </div>
                    <p className="mt-4 text-xs text-muted-foreground">
                      {count(p.id)} ressource(s) · créé {relativeTime(p.created_at)}
                    </p>
                  </Card>
                </Link>
              ))}
            </div>
          )}
        </TabsContent>
        <TabsContent value="variables">
          <VariablesPanel team={team} />
        </TabsContent>
        <TabsContent value="tags">
          <TagsPanel team={team} />
        </TabsContent>
      </Tabs>
      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Nouveau projet</DialogTitle>
          </DialogHeader>
          <form
            id="new-project"
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              create.mutate(form)
            }}
          >
            <Field label="Nom">
              <Input required autoFocus value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </Field>
            <Field label="Description">
              <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </Field>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreating(false)}>
              Annuler
            </Button>
            <Button type="submit" form="new-project" loading={create.isPending}>
              Créer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
