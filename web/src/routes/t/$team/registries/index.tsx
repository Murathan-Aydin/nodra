import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/registries/")({
  component: () => <ComingSoon title="Registres Docker" card="F11-03" />,
})
