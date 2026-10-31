import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/approvals/")({
  component: () => <ComingSoon title="Approbations" card="F3-08" />,
})
