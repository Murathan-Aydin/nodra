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
import type { Environment } from "@/lib/types"
import { ConfirmDelete } from "@/features/projects/confirm-delete"
import { errorText } from "@/features/projects/dialogs"

export const Route = createFileRoute("/t/$team/projects/$project/$env/settings")({ component: EnvironmentSettings })

function EnvironmentSettings() {
  const { team, project, env } = Route.useParams()
  const data = useQuery(projectQueries.environment(team, project, env)).data
  return (
    <>
      <PageHeader
        title="Réglages de l'environnement"
      />
      {data && <EnvironmentForm key={data.environment.name + data.environment.description} team={team} project={project} environment={data.environment} />}
    </>
  )
}

function EnvironmentForm({ team, project, environment }: { team: string; project: string; environment: Environment }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const path = teamPath(team, `/projects/${project}/environments/${environment.id}`)
  const [f, setF] = useState({ name: environment.name, description: environment.description })
  const save = useMutation({
    mutationFn: () => api(path, { method: "PATCH", json: { name: f.name.trim(), description: f.description.trim() } }),
    onSuccess: () => {
      toast.success("Environnement enregistré")
      void qc.invalidateQueries({ queryKey: ["team", team, "project", project] })
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
        what="l'environnement"
        name={environment.name}
        text="Supprime ses variables partagées. Impossible tant qu'il contient des ressources."
        path={path}
        onDeleted={() => {
          void qc.invalidateQueries({ queryKey: ["team", team] })
          void navigate({ to: "/t/$team/projects/$project", params: { team, project } })
        }}
      />
    </div>
  )
}
