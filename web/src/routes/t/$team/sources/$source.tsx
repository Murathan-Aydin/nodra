import { createFileRoute, redirect } from "@tanstack/react-router"

// Adresse de retour de GitHub après l'installation de l'App (fixée par l'API) : page de la source dans Intégrations.
export const Route = createFileRoute("/t/$team/sources/$source")({
  beforeLoad: ({ params, location }) => {
    throw redirect({ href: `/t/${params.team}/integrations/sources/${params.source}${location.searchStr}` })
  },
})
