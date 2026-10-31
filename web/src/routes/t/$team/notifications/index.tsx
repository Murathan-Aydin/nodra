import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/notifications/")({
  component: () => <ComingSoon title="Notifications" card="F11-02" />,
})
