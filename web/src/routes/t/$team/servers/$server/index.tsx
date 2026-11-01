import { createFileRoute } from "@tanstack/react-router"
import { Skeleton } from "@/components/ui/skeleton"
import { useServer } from "@/features/servers/server"
import { ServerGeneral } from "@/features/servers/general"

export const Route = createFileRoute("/t/$team/servers/$server/")({ component: Page })

function Page() {
  const { team, server } = Route.useParams()
  const { server: s } = useServer(team, server)
  if (!s) return <Skeleton className="h-64 rounded-2xl" />
  return <ServerGeneral key={s.id + JSON.stringify(s.settings) + s.name} team={team} server={s} />
}
