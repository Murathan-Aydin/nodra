import { createFileRoute } from "@tanstack/react-router"
import { ComingSoon } from "@/components/coming-soon"
import { PageHeader } from "@/components/page-header"
import { CreateApplication } from "@/features/applications/create-app"

type ResourceType = "application" | "database" | "service"

export const Route = createFileRoute("/t/$team/projects/$project/$env/new")({
  validateSearch: (s: Record<string, unknown>): { type: ResourceType } => ({
    type: s.type === "database" || s.type === "service" ? s.type : "application",
  }),
  component: NewResource,
})

function NewResource() {
  const { team, env } = Route.useParams()
  const { type } = Route.useSearch()
  if (type === "application")
    return (
      <>
        <PageHeader title="Nouvelle application" description="Depuis un dépôt Git, une image Docker ou un docker-compose." />
        <CreateApplication team={team} environment={env} />
      </>
    )
  return <ComingSoon title="Nouvelle ressource" card={type === "database" ? "F9-01" : "F10-02"} />
}
