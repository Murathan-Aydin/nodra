import { createFileRoute } from "@tanstack/react-router"
import { PageHeader } from "@/components/page-header"
import { CreateApplication } from "@/features/applications/create-app"
import { CreateDatabase } from "@/features/databases/create-db"
import { CreateService } from "@/features/services/create-service"

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
  if (type === "database")
    return (
      <>
        <PageHeader title="Nouvelle base de données" description="PostgreSQL, MySQL, MariaDB, MongoDB, Redis, KeyDB, Dragonfly ou ClickHouse." />
        <CreateDatabase team={team} environment={env} />
      </>
    )
  return (
    <>
      <PageHeader title="Nouveau service" description="Plus de 340 services prêts à l'emploi, ou votre propre docker-compose." />
      <CreateService team={team} environment={env} />
    </>
  )
}
