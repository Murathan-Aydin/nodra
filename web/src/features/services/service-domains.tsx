import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api } from "@/lib/api"
import type { ServiceApp } from "@/lib/types"
import { displayName } from "./compose-resources"
import { useService } from "./use-service"

/** Onglet « Domaines » : domaines de chaque sous-service exposé (PATCH /services/{id}/apps/{nom}). */
export function ServiceDomains({ team, service }: { team: string; service: string }) {
  const { apps } = useService(team, service)
  return (
    <div className="space-y-6">
      {apps.map((a) => (
        <AppDomains key={a.id + (a.domains ?? []).join()} team={team} service={service} app={a} />
      ))}
    </div>
  )
}

function AppDomains({ team, service, app }: { team: string; service: string; app: ServiceApp }) {
  const qc = useQueryClient()
  const initial = (app.domains ?? []).join(", ")
  const [domains, setDomains] = useState(initial)
  const [port, setPort] = useState(app.port)
  const save = useMutation({
    mutationFn: () =>
      api(`/teams/${team}/services/${service}/apps/${app.name}`, {
        method: "PATCH",
        json: { domains: domains.split(/[\s,]+/).filter(Boolean), port },
      }),
    onSuccess: () => {
      toast.success("Domaines enregistrés", { description: "Appliqués au prochain démarrage du service." })
      void qc.invalidateQueries({ queryKey: ["team", team, "service", service] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title={displayName(app.name)}
      description={app.image}
      actions={
        <Button size="sm" variant="outline" disabled={domains === initial && port === app.port} loading={save.isPending} onClick={() => save.mutate()}>
          Enregistrer
        </Button>
      }
    >
      <div className="grid gap-4 sm:grid-cols-[1fr_10rem]">
        <Field label="Domaines" hint="Séparés par des virgules. Le port du conteneur peut être indiqué : https://app.exemple.fr:3000">
          <Input value={domains} onChange={(e) => setDomains(e.target.value)} placeholder="https://app.exemple.fr" className="font-mono" />
        </Field>
        <Field label="Port du conteneur">
          <Input value={port} onChange={(e) => setPort(e.target.value)} className="font-mono" />
        </Field>
      </div>
    </SettingsCard>
  )
}
