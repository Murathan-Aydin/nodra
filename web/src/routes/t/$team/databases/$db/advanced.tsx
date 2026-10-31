import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { patch } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Database, Tag } from "@/lib/types"
import { dbBase, dbQuery } from "@/features/databases/queries"
import { ResourceActions } from "@/features/resources/resource-actions"
import { TagPicker } from "@/features/resources/tag-picker"

export const Route = createFileRoute("/t/$team/databases/$db/advanced")({ component: DatabaseAdvanced })

function DatabaseAdvanced() {
  const { team, db } = Route.useParams()
  const { data } = useQuery(dbQuery(team, db))
  if (!data) return null
  return <AdvancedForm key={data.database.updated_at} team={team} d={data.database} tags={data.tags} />
}

function AdvancedForm({ team, d, tags }: { team: string; d: Database; tags: Tag[] }) {
  const canUpdate = usePermission(team, "resource.update", { type: "resource", id: d.id })
  const base = dbBase(team, d.id)
  const [f, setF] = useState({ ports_mappings: d.ports_mappings, custom_docker_run_options: d.custom_docker_run_options, log_drain_enabled: d.settings.log_drain_enabled })
  const save = useApiMutation(
    () =>
      patch(base, {
        ports_mappings: f.ports_mappings,
        custom_docker_run_options: f.custom_docker_run_options,
        settings: { log_drain_enabled: f.log_drain_enabled },
      }),
    { success: "Réglages enregistrés (appliqués au prochain démarrage)", invalidate: [["team", team, "db", d.id]] },
  )
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Conteneur</CardTitle>
          <CardDescription>Appliqué au prochain démarrage.</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              save.mutate(undefined)
            }}
          >
            <fieldset disabled={!canUpdate} className="grid gap-4 md:grid-cols-2">
              <Field label="Ports publiés sur l'hôte" hint={`ex. 5433:${d.internal_port} (sans proxy, préférer l'accès public)`}>
                <Input className="font-mono" value={f.ports_mappings} onChange={(e) => setF({ ...f, ports_mappings: e.target.value })} />
              </Field>
              <Field label="Options docker run" hint="--cap-add, --ulimit, --shm-size…">
                <Input className="font-mono" value={f.custom_docker_run_options} onChange={(e) => setF({ ...f, custom_docker_run_options: e.target.value })} />
              </Field>
              <Label className="flex items-center justify-between font-normal md:col-span-2">
                Envoyer les logs au log drain du serveur
                <Switch checked={f.log_drain_enabled} onCheckedChange={(v) => setF({ ...f, log_drain_enabled: v })} disabled={!canUpdate} />
              </Label>
            </fieldset>
            {canUpdate && (
              <div className="flex justify-end">
                <Button type="submit" loading={save.isPending}>
                  Enregistrer
                </Button>
              </div>
            )}
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Tags</CardTitle>
        </CardHeader>
        <CardContent>
          <TagPicker team={team} base={base} current={tags} invalidate={[["team", team, "db", d.id]]} disabled={!canUpdate} />
        </CardContent>
      </Card>
      <ResourceActions team={team} kind="database" id={d.id} name={d.name} base={base} environmentId={d.environment_id} destinationId={d.destination_id} />
    </div>
  )
}
