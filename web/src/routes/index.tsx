import { createFileRoute, redirect } from "@tanstack/react-router"
import { teamsQuery } from "@/lib/queries"
import { requireSession } from "@/lib/session"

// Racine : tableau de bord de l'équipe personnelle (ou de la première équipe).
export const Route = createFileRoute("/")({
  beforeLoad: async ({ context }) => {
    await requireSession(context.queryClient, "/")
    const teams = await context.queryClient.ensureQueryData(teamsQuery)
    const team = teams.find((t) => t.personal) ?? teams[0]
    if (team) throw redirect({ to: "/t/$team", params: { team: team.id } })
  },
})
