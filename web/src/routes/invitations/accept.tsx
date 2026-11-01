import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { AuthPanel, AuthShell } from "@/components/auth-shell"
import { Button } from "@/components/ui/button"
import { post } from "@/lib/api"
import { dateTime } from "@/lib/format"
import { meQuery } from "@/lib/queries"
import type { Team } from "@/lib/types"

export const Route = createFileRoute("/invitations/accept")({
  validateSearch: (s: Record<string, unknown>): { token?: string } => (typeof s.token === "string" ? { token: s.token } : {}),
  component: Accept,
})

/** Acceptation d'une invitation : connexion ou inscription d'abord, avec la même adresse. */
function Accept() {
  const { token } = Route.useSearch()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const me = useQuery({ ...meQuery, retry: false })
  const here = `/invitations/accept?token=${token ?? ""}`
  const preview = useQuery({
    queryKey: ["invitation", token],
    queryFn: () => post<{ team_name: string; role_name: string; email: string; expires_at: string }>("/invitations/preview", { token }),
    enabled: !!token && me.isSuccess,
    retry: false,
  })
  const accept = useMutation({
    mutationFn: () => post<Team>("/invitations/accept", { token }),
    onSuccess: async (t) => {
      await qc.invalidateQueries({ queryKey: ["teams"] })
      void navigate({ to: "/t/$team", params: { team: t.id } })
    },
  })
  return (
    <AuthShell>
      <AuthPanel title="Invitation">
        {!token ? (
          <p className="text-sm text-destructive">Lien incomplet : jeton manquant.</p>
        ) : me.isPending ? null : !me.data ? (
          <div className="space-y-3">
            <p className="text-sm">Connectez-vous (ou créez un compte) avec l'adresse invitée pour rejoindre l'équipe.</p>
            <Button className="w-full" asChild>
              <Link to="/login" search={{ redirect: here }}>
                Se connecter
              </Link>
            </Button>
            <Button variant="outline" className="w-full" asChild>
              <Link to="/register" search={{ redirect: here }}>
                Créer un compte
              </Link>
            </Button>
          </div>
        ) : preview.data ? (
          <div className="space-y-4">
            <p className="text-sm">
              Vous êtes invité dans <strong>{preview.data.team_name}</strong> avec le rôle <strong>{preview.data.role_name}</strong> ({preview.data.email}, valable jusqu'au {dateTime(preview.data.expires_at)}).
            </p>
            {accept.error && <p className="text-sm text-destructive">{accept.error.message}</p>}
            <Button className="w-full" loading={accept.isPending} onClick={() => accept.mutate()}>
              Rejoindre l'équipe
            </Button>
          </div>
        ) : (
          <p className="text-sm text-destructive">{preview.error?.message ?? "Chargement…"}</p>
        )}
      </AuthPanel>
    </AuthShell>
  )
}
