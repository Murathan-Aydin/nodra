import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"

type ResourceType = "application" | "database" | "service"

export const Route = createFileRoute("/t/$team/projects/$project/$env/new")({
  validateSearch: (s: Record<string, unknown>): { type: ResourceType } => ({
    type: s.type === "database" || s.type === "service" ? s.type : "application",
  }),
  component: NewResource,
})

function NewResource() {
  const { type } = Route.useSearch()
  const card = type === "application" ? "F8-01" : type === "database" ? "F9-01" : "F10-02"
  return <ComingSoon title="Nouvelle ressource" card={card} />
}
