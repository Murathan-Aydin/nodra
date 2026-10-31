import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { PageHeader, Section } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api, teamPath } from "@/lib/api"
import { projectQueries } from "@/lib/queries"
import type { ProjectSummary } from "@/lib/types"
import { ConfirmDelete } from "@/features/projects/confirm-delete"
import { errorText } from "@/features/projects/dialogs"

export const Route = createFileRoute("/t/$team/projects/$project/settings")({ component: ProjectSettings })

function ProjectSettings() {
  const { team, project } = Route.useParams()
  const data = useQuery(projectQueries.project(team, project)).data
  return (
    <>
      <PageHeader
        crumbs={[{ label: "Projets", to: `/t/${team}/projects` }, { label: data?.project.name ?? "…", to: `/t/${team}/projects/${project}` }, { label: "Réglages" }]}
        title="Réglages du projet"
      />
      {data && <ProjectForm key={data.project.name + data.project.description} team={team} project={data.project} />}
    </>
  )
}

function ProjectForm({ team, project }: { team: string; project: ProjectSummary }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [f, setF] = useState({ name: project.name, description: project.description })
  const save = useMutation({
    mutationFn: () => api(teamPath(team, `/projects/${project.id}`), { method: "PATCH", json: { name: f.name.trim(), description: f.description.trim() } }),
    onSuccess: () => {
      toast.success("Projet enregistré")
      void qc.invalidateQueries({ queryKey: ["team", team] })
    },
    onError: (e) => toast.error(errorText(e, "Enregistrement impossible")),
  })
  return (
    <div className="max-w-3xl space-y-6">
      <Section title="Général">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate()
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Nom">
              <Input required value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
            </Field>
            <Field label="Description">
              <Input value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
            </Field>
          </div>
          <div className="flex justify-end">
            <Button type="submit" loading={save.isPending} disabled={!f.name.trim()}>
              Enregistrer
            </Button>
          </div>
        </form>
      </Section>
      <ConfirmDelete
        what="le projet"
        name={project.name}
        text="Supprime ses environnements, variables et droits. Impossible tant qu'il contient des ressources."
        path={teamPath(team, `/projects/${project.id}`)}
        onDeleted={() => {
          void qc.invalidateQueries({ queryKey: ["team", team, "projects"] })
          void navigate({ to: "/t/$team/projects", params: { team } })
        }}
      />
    </div>
  )
}
