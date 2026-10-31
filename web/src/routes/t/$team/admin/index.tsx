import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/admin/")({
  component: () => <ComingSoon title="Administration" card="F12-01" />,
})
