import { createFileRoute } from "@tanstack/react-router"
import { StoragesPanel } from "@/features/resources/storages-panel"
import { dbBase } from "@/features/databases/queries"

export const Route = createFileRoute("/t/$team/databases/$db/storages")({ component: DatabaseStorages })

function DatabaseStorages() {
  const { team, db } = Route.useParams()
  return <StoragesPanel team={team} base={dbBase(team, db)} resource={db} />
}
