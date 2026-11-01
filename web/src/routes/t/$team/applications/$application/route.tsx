import { Outlet, createFileRoute } from "@tanstack/react-router"
import { TopBarActions } from "@/components/app-shell"
import { ApplicationActions } from "@/features/applications/application-actions"
import { useApplication } from "@/features/applications/use-application"
import { resourceMenu } from "@/features/resources/resource"

// Pages d'une application : menu latéral de l'application et actions dans la barre du haut.
export const Route = createFileRoute("/t/$team/applications/$application")({
  staticData: { nav: (p) => resourceMenu({ team: p.team, kind: "application", id: p.application }) },
  component: ApplicationLayout,
})

function ApplicationLayout() {
  const { team, application } = Route.useParams()
  const { ref, app } = useApplication(team, application)
  return (
    <>
      <TopBarActions>{app && <ApplicationActions resource={ref} app={app} />}</TopBarActions>
      <Outlet />
    </>
  )
}
