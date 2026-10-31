import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/audit/")({
  component: () => <ComingSoon title="Journal d'audit" card="F3-07" />,
})
