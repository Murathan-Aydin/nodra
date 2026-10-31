import { createFileRoute } from "@tanstack/react-router"
import { StoragesPanel } from "@/features/resources/storages-panel"
import { appBase } from "@/features/applications/queries"

export const Route = createFileRoute("/t/$team/applications/$app/storages")({ component: AppStorages })

function AppStorages() {
  const { team, app } = Route.useParams()
  return <StoragesPanel team={team} base={appBase(team, app)} resource={app} />
}
