import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { ExternalLinkIcon } from "lucide-react"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { instanceQuery } from "@/lib/queries"
import type { ResourceRef } from "./resource"

/**
 * « Deploy webhook » du fork : déclenche le déploiement (ou le démarrage) depuis un service externe,
 * avec un jeton API de l'équipe (POST /teams/{team}/deploy?uuid=…).
 */
export function DeployWebhookCard({ resource }: { resource: ResourceRef }) {
  const url = `${location.origin}/api/v1/teams/${resource.team}/deploy?uuid=${resource.id}&force=false`
  const api = useQuery(instanceQuery).data
  return (
    <SettingsCard
      title="Webhook de déploiement"
      description="Déclenche un déploiement depuis un service externe. La requête doit porter un jeton API valide (Authorization: Bearer)."
      actions={
        <Button size="sm" variant="outline" asChild>
          <Link to="/t/$team/keys/tokens" params={{ team: resource.team }}>
            Jetons API <ExternalLinkIcon />
          </Link>
        </Button>
      }
    >
      {api?.api_enabled === false && <p className="mb-4 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-sm">L'API par jetons est désactivée dans les réglages de l'instance : activez-la pour que les services externes puissent appeler ce webhook.</p>}
      <div className="space-y-4">
        <Field label="URL du webhook de déploiement">
          <CopyInput value={url} />
        </Field>
        <Field label="Exemple">
          <CopyInput value={`curl -X POST -H "Authorization: Bearer <jeton>" "${url}"`} />
        </Field>
      </div>
    </SettingsCard>
  )
}

/** Onglet « Webhooks » des bases et des services. */
export function ResourceWebhooksTab({ resource }: { resource: ResourceRef }) {
  return <DeployWebhookCard resource={resource} />
}
