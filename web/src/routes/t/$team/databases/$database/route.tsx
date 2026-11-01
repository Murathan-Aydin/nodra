import { Outlet, createFileRoute } from "@tanstack/react-router"
import { TopBarActions } from "@/components/app-shell"
import { useDatabase } from "@/features/databases/use-database"
import { LinksMenu, LifecycleActions } from "@/features/resources/resource-actions"
import { resourceMenu } from "@/features/resources/resource"

// Pages d'une base : menu latéral de la base et actions dans la barre du haut.
export const Route = createFileRoute("/t/$team/databases/$database")({
  staticData: { nav: (p) => resourceMenu({ team: p.team, kind: "database", id: p.database }) },
  component: DatabaseLayout,
})

function DatabaseLayout() {
  const { team, database } = Route.useParams()
  const { ref, db } = useDatabase(team, database)
  return (
    <>
      <TopBarActions>
        {db && (
          <>
            <LinksMenu links={db.public_url ? [{ label: "public", url: db.public_url }] : []} />
            <LifecycleActions resource={ref} status={db.status} />
          </>
        )}
      </TopBarActions>
      <Outlet />
    </>
  )
}
