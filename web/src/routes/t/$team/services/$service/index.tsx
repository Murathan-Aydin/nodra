import { createFileRoute } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { api } from "@/lib/api"
import { serviceQueries } from "@/lib/queries"
import { ComposeResources } from "@/features/services/compose-resources"
import { ServiceConfig } from "@/features/services/service-config"
import type { Service } from "@/lib/types"

export const Route = createFileRoute("/t/$team/services/$service/")({ component: ServicePage })

function ServicePage() {
  const { team, service } = Route.useParams()
  const data = useQuery(serviceQueries.service(team, service)).data

  return (
    <div className="space-y-6">
      {data ? <ServiceDetails key={data.service.name + data.service.description} team={team} service={data.service} /> : <Skeleton className="h-56 rounded-2xl" />}
      <ServiceConfig team={team} service={service} />
      {data && <ComposeResources team={team} service={data.service} apps={data.apps} />}
    </div>
  )
}

/** « Service details » : nom et description du service. */
function ServiceDetails({ team, service }: { team: string; service: Service }) {
  const qc = useQueryClient()
  const [name, setName] = useState(service.name)
  const [description, setDescription] = useState(service.description)
  const save = useMutation({
    mutationFn: () => api(`/teams/${team}/services/${service.id}`, { method: "PATCH", json: { name, description } }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["team", team, "service", service.id] })
      void qc.invalidateQueries({ queryKey: ["team", team, "resources"] })
    },
  })
  const dirty = name !== service.name || description !== service.description

  return (
    <SettingsCard
      title="Détails du service"
      actions={
        <Button size="sm" variant="outline" disabled={!dirty || !name.trim() || save.isPending} onClick={() => save.mutate()}>
          Enregistrer
        </Button>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Nom du service *" error={save.error?.message}>
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Description">
          <Input value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
      </div>
    </SettingsCard>
  )
}
