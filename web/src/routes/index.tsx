import { createFileRoute, redirect } from "@tanstack/react-router"
import { ApiError } from "@/lib/api"
import { meQuery, teamsQuery } from "@/lib/queries"

// Accueil : connexion si nécessaire, sinon tableau de bord de l'équipe personnelle.
export const Route = createFileRoute("/")({
  beforeLoad: async ({ context }) => {
    try {
      await context.queryClient.ensureQueryData(meQuery)
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) throw redirect({ to: "/login" })
      throw e
    }
    const teams = await context.queryClient.ensureQueryData(teamsQuery)
    const team = teams.find((t) => t.personal) ?? teams[0]
    throw redirect({ to: "/t/$team", params: { team: team.id } })
  },
})
