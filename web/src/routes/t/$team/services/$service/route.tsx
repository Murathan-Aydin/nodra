import { Outlet, createFileRoute } from "@tanstack/react-router"
import { TopBarActions } from "@/components/app-shell"
import { ServiceActions } from "@/features/resources/resource-actions"
import { resourceMenu } from "@/features/resources/resource"
import { useService } from "@/features/services/use-service"

// Pages d'un service : menu latéral du service et actions dans la barre du haut, sur tous les onglets.
export const Route = createFileRoute("/t/$team/services/$service")({
  staticData: { nav: (p) => resourceMenu({ team: p.team, kind: "service", id: p.service }) },
  component: ServiceLayout,
})

function ServiceLayout() {
  const { team, service } = Route.useParams()
  const { data } = useService(team, service)
  return (
    <>
      <TopBarActions>{data && <ServiceActions team={team} service={data.service} apps={data.apps} />}</TopBarActions>
      <Outlet />
    </>
  )
}
