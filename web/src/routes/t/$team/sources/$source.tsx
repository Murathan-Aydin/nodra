import { createFileRoute, redirect } from "@tanstack/react-router"

// Adresse de retour de GitHub après l'installation de l'App (fixée par l'API) : page de la source dans Intégrations.
export const Route = createFileRoute("/t/$team/sources/$source")({
  beforeLoad: ({ params, search }) => {
    throw redirect({ to: "/t/$team/integrations/sources/$source", params, search })
  },
})
