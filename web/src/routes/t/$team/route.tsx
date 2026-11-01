import { Outlet, createFileRoute } from "@tanstack/react-router"
import { AppShell } from "@/components/app-shell"
import { requireSession } from "@/lib/session"
import { useTeamEvents } from "@/lib/socket"

export const Route = createFileRoute("/t/$team")({
  beforeLoad: ({ context, location }) => requireSession(context.queryClient, location.href),
  component: TeamLayout,
})

function TeamLayout() {
  const { team } = Route.useParams()
  useTeamEvents(team)
  return (
    <AppShell team={team}>
      <Outlet />
    </AppShell>
  )
}
