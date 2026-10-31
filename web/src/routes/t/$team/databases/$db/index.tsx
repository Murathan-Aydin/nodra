import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

export const Route = createFileRoute("/t/$team/databases/$db/")({
  component: () => <ComingSoon title="Base de données" card="F9-02" />,
})
