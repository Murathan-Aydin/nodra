import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { patch } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { ResourceActions } from "@/features/resources/resource-actions"
import { TagPicker } from "@/features/resources/tag-picker"
import { svcBase, svcQuery } from "@/features/services/queries"

export const Route = createFileRoute("/t/$team/services/$svc/advanced")({ component: ServiceAdvanced })

function ServiceAdvanced() {
  const { team, svc } = Route.useParams()
  const { data } = useQuery(svcQuery(team, svc))
  const canUpdate = usePermission(team, "resource.update", { type: "resource", id: svc })
  const base = svcBase(team, svc)
  const key = ["team", team, "svc", svc]
  const drain = useApiMutation((v: boolean) => patch(base, { log_drain_enabled: v }), {
    success: (_, v) => (v ? "Logs envoyés au log drain (prochain démarrage)" : "Log drain désactivé (prochain démarrage)"),
    invalidate: [key],
  })
  if (!data) return null
  const s = data.service
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Logs</CardTitle>
        </CardHeader>
        <CardContent>
          <Label className="flex items-center justify-between font-normal">
            Envoyer les logs au log drain du serveur
            <Switch checked={s.log_drain_enabled} onCheckedChange={(v) => drain.mutate(v)} disabled={!canUpdate || drain.isPending} />
          </Label>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Tags</CardTitle>
        </CardHeader>
        <CardContent>
          <TagPicker team={team} base={base} current={data.tags} invalidate={[key]} disabled={!canUpdate} />
        </CardContent>
      </Card>
      <ResourceActions team={team} kind="service" id={s.id} name={s.name} base={base} environmentId={s.environment_id} destinationId={s.destination_id} />
    </div>
  )
}
