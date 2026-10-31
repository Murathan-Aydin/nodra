import { createFileRoute } from "@tanstack/react-router"
import { BackupsPanel } from "@/features/backups/backups-panel"

export const Route = createFileRoute("/t/$team/services/$svc/backups")({ component: ServiceBackups })

function ServiceBackups() {
  const { team, svc } = Route.useParams()
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">Archives des volumes Docker du service (nom visible dans le compose, préfixé par l'identifiant du service).</p>
      <BackupsPanel team={team} resource={svc} database={false} />
    </div>
  )
}
