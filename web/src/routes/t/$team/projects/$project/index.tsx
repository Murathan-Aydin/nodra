import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { CopyIcon, LayersIcon, MoreHorizontalIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { del, patch, post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Environment } from "@/lib/types"
import { projectQuery, projectsQuery, resourcesQuery } from "@/features/projects/queries"
import { VariablesPanel } from "@/features/projects/variables-panel"

export const Route = createFileRoute("/t/$team/projects/$project/")({ component: ProjectPage })

type EnvDialog = { mode: "create" } | { mode: "rename"; env: Environment } | { mode: "clone"; env: Environment }

function ProjectPage() {
  const { team, project } = Route.useParams()
  const navigate = useNavigate()
  const { data, isPending } = useQuery(projectQuery(team, project))
  const projects = useQuery(projectsQuery(team))
  const resources = useQuery(resourcesQuery(team))
  const scope = { type: "project", id: project }
  const canCreateEnv = usePermission(team, "environment.create", scope)
  const canUpdateEnv = usePermission(team, "environment.update", scope)
  const canDeleteEnv = usePermission(team, "environment.delete", scope)
  const canUpdate = usePermission(team, "project.update", scope)
  const canDelete = usePermission(team, "project.delete", scope)
  const [dialog, setDialog] = useState<EnvDialog | null>(null)
  const [form, setForm] = useState({ name: "", description: "", project_id: project, with_resources: false })
  const [deletingEnv, setDeletingEnv] = useState<Environment | null>(null)
  const [deletingProject, setDeletingProject] = useState(false)
  const [editingProject, setEditingProject] = useState(false)
  const [projectForm, setProjectForm] = useState({ name: "", description: "" })
  const key = projectQuery(team, project).queryKey
  const base = teamPath(team, `/projects/${project}`)

  const saveEnv = useApiMutation(
    () => {
      if (dialog?.mode === "rename") return patch(`${base}/environments/${dialog.env.id}`, { name: form.name, description: form.description })
      if (dialog?.mode === "clone") return post<Environment>(`${base}/environments/${dialog.env.id}/clone`, form)
      return post<Environment>(`${base}/environments`, { name: form.name, description: form.description })
    },
    {
      success: dialog?.mode === "clone" ? "Environnement cloné" : "Environnement enregistré",
      invalidate: [key, ["team", team, "resources"], ["team", team, "project"]],
      onSuccess: () => setDialog(null),
    },
  )
  const removeEnv = useApiMutation((e: Environment) => del(`${base}/environments/${e.id}`), {
    success: (r) => (r ? undefined : "Environnement supprimé"),
    invalidate: [key],
    onSuccess: () => setDeletingEnv(null),
  })
  const updateProject = useApiMutation(() => patch(base, projectForm), {
    success: "Projet mis à jour",
    invalidate: [key, projectsQuery(team).queryKey],
    onSuccess: () => setEditingProject(false),
  })
  const removeProject = useApiMutation(() => del(base), {
    success: (r) => (r ? undefined : "Projet supprimé"),
    invalidate: [projectsQuery(team).queryKey],
    onSuccess: (r) => {
      setDeletingProject(false)
      if (!r) void navigate({ to: "/t/$team/projects", params: { team } })
    },
  })

  const open = (d: EnvDialog) => {
    setForm({
      name: d.mode === "rename" ? d.env.name : d.mode === "clone" ? `${d.env.name}-copie` : "",
      description: d.mode === "rename" ? d.env.description : "",
      project_id: project,
      with_resources: false,
    })
    setDialog(d)
  }
  if (isPending || !data) return <Skeleton className="h-40 w-full" />
  const count = (env: string) => resources.data?.filter((r) => r.environment_id === env).length ?? 0

  return (
    <>
      <PageHeader
        title={data.project.name}
        description={data.project.description || "Projet"}
        actions={
          <>
            {canCreateEnv && (
              <Button onClick={() => open({ mode: "create" })}>
                <PlusIcon /> Environnement
              </Button>
            )}
            {(canUpdate || canDelete) && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" size="icon" aria-label="Plus d'actions">
                    <MoreHorizontalIcon />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {canUpdate && (
                    <DropdownMenuItem
                      onSelect={() => {
                        setProjectForm({ name: data.project.name, description: data.project.description })
                        setEditingProject(true)
                      }}
                    >
                      <PencilIcon /> Renommer
                    </DropdownMenuItem>
                  )}
                  {canDelete && (
                    <DropdownMenuItem variant="destructive" onSelect={() => setDeletingProject(true)}>
                      <Trash2Icon /> Supprimer le projet
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </>
        }
      />
      <Tabs defaultValue="envs">
        <TabsList>
          <TabsTrigger value="envs">Environnements</TabsTrigger>
          <TabsTrigger value="variables">Variables du projet</TabsTrigger>
        </TabsList>
        <TabsContent value="envs">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {data.environments.map((env) => (
              <Card key={env.id} className="transition-colors hover:border-primary/40">
                <CardHeader className="flex-row items-start justify-between gap-2">
                  <Link to="/t/$team/projects/$project/$env" params={{ team, project, env: env.id }} className="min-w-0 flex-1">
                    <CardTitle className="flex items-center gap-2">
                      <LayersIcon className="size-4 text-muted-foreground" />
                      {env.name}
                    </CardTitle>
                    {env.description && <p className="mt-1 text-sm text-muted-foreground">{env.description}</p>}
                  </Link>
                  {(canUpdateEnv || canCreateEnv || canDeleteEnv) && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-sm" aria-label="Actions">
                          <MoreHorizontalIcon />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {canUpdateEnv && (
                          <DropdownMenuItem onSelect={() => open({ mode: "rename", env })}>
                            <PencilIcon /> Renommer
                          </DropdownMenuItem>
                        )}
                        {canCreateEnv && (
                          <DropdownMenuItem onSelect={() => open({ mode: "clone", env })}>
                            <CopyIcon /> Cloner
                          </DropdownMenuItem>
                        )}
                        {canDeleteEnv && (
                          <DropdownMenuItem variant="destructive" onSelect={() => setDeletingEnv(env)}>
                            <Trash2Icon /> Supprimer
                          </DropdownMenuItem>
                        )}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </CardHeader>
                <CardContent>
                  <Link to="/t/$team/projects/$project/$env" params={{ team, project, env: env.id }} className="text-sm text-muted-foreground hover:text-foreground">
                    {count(env.id)} ressource(s) →
                  </Link>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>
        <TabsContent value="variables">
          <VariablesPanel team={team} scope={{ type: "project", id: project }} />
        </TabsContent>
      </Tabs>

      <Dialog open={!!dialog} onOpenChange={(o) => !o && setDialog(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {dialog?.mode === "create" ? "Nouvel environnement" : dialog?.mode === "rename" ? "Renommer l'environnement" : `Cloner ${dialog?.mode === "clone" ? dialog.env.name : ""}`}
            </DialogTitle>
            {dialog?.mode === "clone" && <DialogDescription>Les variables sont copiées ; les ressources en option (arrêtées, nouveaux domaines et volumes vides).</DialogDescription>}
          </DialogHeader>
          <form
            id="env-form"
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              saveEnv.mutate(undefined)
            }}
          >
            <Field label="Nom">
              <Input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </Field>
            {dialog?.mode !== "clone" && (
              <Field label="Description">
                <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </Field>
            )}
            {dialog?.mode === "clone" && (
              <>
                <Field label="Projet cible">
                  <Select value={form.project_id} onValueChange={(v) => setForm({ ...form, project_id: v })}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {projects.data?.map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Label className="font-normal">
                  <Checkbox checked={form.with_resources} onCheckedChange={(c) => setForm({ ...form, with_resources: c === true })} /> Copier aussi les ressources
                </Label>
              </>
            )}
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialog(null)}>
              Annuler
            </Button>
            <Button type="submit" form="env-form" loading={saveEnv.isPending}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={editingProject} onOpenChange={setEditingProject}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Projet</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <Field label="Nom">
              <Input value={projectForm.name} onChange={(e) => setProjectForm({ ...projectForm, name: e.target.value })} />
            </Field>
            <Field label="Description">
              <Input value={projectForm.description} onChange={(e) => setProjectForm({ ...projectForm, description: e.target.value })} />
            </Field>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingProject(false)}>
              Annuler
            </Button>
            <Button loading={updateProject.isPending} onClick={() => updateProject.mutate(undefined)}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!deletingEnv}
        onOpenChange={(o) => !o && setDeletingEnv(null)}
        title={`Supprimer l'environnement ${deletingEnv?.name} ?`}
        description="Impossible s'il contient encore des ressources. Peut exiger une approbation."
        confirmText={deletingEnv?.name}
        confirmLabel="Supprimer"
        destructive
        loading={removeEnv.isPending}
        onConfirm={() => deletingEnv && removeEnv.mutate(deletingEnv)}
      />
      <ConfirmDialog
        open={deletingProject}
        onOpenChange={setDeletingProject}
        title={`Supprimer le projet ${data.project.name} ?`}
        description="Ses environnements et variables sont supprimés ; impossible s'il reste des ressources."
        confirmText={data.project.name}
        confirmLabel="Supprimer"
        destructive
        loading={removeProject.isPending}
        onConfirm={() => removeProject.mutate(undefined)}
      />
    </>
  )
}
