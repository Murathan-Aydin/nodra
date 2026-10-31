import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { UsersIcon } from "lucide-react"
import { AuthLayout } from "@/components/auth-layout"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { ApiError, post } from "@/lib/api"
import { meQuery, teamsQuery } from "@/lib/queries"
import type { Team } from "@/lib/types"

export const Route = createFileRoute("/invitations/accept")({
  validateSearch: (s: Record<string, unknown>) => ({ token: typeof s.token === "string" ? s.token : "" }),
  component: AcceptInvitation,
})

function AcceptInvitation() {
  const { token } = Route.useSearch()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const me = useQuery({ ...meQuery, retry: false })
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(false)
  const back = `/invitations/accept?token=${encodeURIComponent(token)}`

  async function accept() {
    setLoading(true)
    setError(undefined)
    try {
      const team = await post<Team>("/invitations/accept", { token })
      await qc.invalidateQueries({ queryKey: teamsQuery.queryKey })
      await navigate({ to: "/t/$team", params: { team: team.id } })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Acceptation impossible")
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout title="Invitation" description="Vous êtes invité à rejoindre une équipe sur Nodra.">
      {me.isPending ? (
        <p className="text-sm text-muted-foreground">Chargement…</p>
      ) : me.isError ? (
        <div className="space-y-3">
          <p className="text-sm text-muted-foreground">Connectez-vous ou créez un compte avec l'adresse e-mail invitée, puis revenez ici.</p>
          <Button asChild className="w-full">
            <Link to="/login" search={{ redirect: back }}>
              Se connecter
            </Link>
          </Button>
          <Button asChild variant="outline" className="w-full">
            <Link to="/register" search={{ redirect: back }}>
              Créer un compte
            </Link>
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <p className="text-sm">
            Connecté en tant que <strong>{me.data.user.email}</strong>.
          </p>
          <Button className="w-full" onClick={accept} loading={loading} disabled={!token}>
            <UsersIcon /> Rejoindre l'équipe
          </Button>
        </div>
      )}
    </AuthLayout>
  )
}
