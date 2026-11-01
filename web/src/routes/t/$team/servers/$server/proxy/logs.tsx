import { createFileRoute } from "@tanstack/react-router"
import { SettingsCard } from "@/components/page"
import { LogStream } from "@/features/resources/logs"

export const Route = createFileRoute("/t/$team/servers/$server/proxy/logs")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  return (
    <SettingsCard title="Logs du proxy" description="Conteneur nodra-proxy, en direct." bodyClassName="p-0">
      <LogStream path={`/teams/${team}/servers/${server}/proxy/logs/stream?tail=200`} />
    </SettingsCard>
  )
}
