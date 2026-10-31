import { createFileRoute, redirect } from "@tanstack/react-router"
import { instanceQuery, teamsQuery } from "@/lib/queries"
import { requireSession } from "@/lib/session"

// Racine : configuration guidée en attente, sinon tableau de bord de l'équipe personnelle (ou de la première).
export const Route = createFileRoute("/")({
  beforeLoad: async ({ context }) => {
    const me = await requireSession(context.queryClient, "/")
    // Configuration guidée pas encore terminée ni passée : on y retourne (administrateur uniquement).
    const instance = await context.queryClient.fetchQuery(instanceQuery)
    if (instance.onboarding && me.user.is_instance_admin) throw redirect({ to: "/onboarding" })
    const teams = await context.queryClient.ensureQueryData(teamsQuery)
    const team = teams.find((t) => t.personal) ?? teams[0]
    if (team) throw redirect({ to: "/t/$team", params: { team: team.id } })
  },
})
