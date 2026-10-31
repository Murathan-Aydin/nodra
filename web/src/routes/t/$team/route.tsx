import { Outlet, createFileRoute } from "@tanstack/react-router"
import { AppShell } from "@/components/app-shell"
import { requireSession } from "@/lib/session"

export const Route = createFileRoute("/t/$team")({
  beforeLoad: ({ context, location }) => requireSession(context.queryClient, location.href),
  component: TeamLayout,
})

function TeamLayout() {
  const { team } = Route.useParams()
  return (
    <AppShell team={team}>
      <Outlet />
    </AppShell>
  )
}
