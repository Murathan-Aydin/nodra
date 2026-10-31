import { createFileRoute, Outlet, redirect } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { MailWarningIcon } from "lucide-react"
import { AppShell } from "@/components/app-shell"
import { Button } from "@/components/ui/button"
import { ApiError, post } from "@/lib/api"
import { meQuery, teamQuery } from "@/lib/queries"
import { useApiMutation } from "@/lib/mutations"

// Zone connectée : session requise, équipe accessible, puis mise en page avec barre latérale.
export const Route = createFileRoute("/t/$team")({
  beforeLoad: async ({ context, params, location }) => {
    try {
      await context.queryClient.ensureQueryData(meQuery)
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) throw redirect({ to: "/login", search: { redirect: location.href } })
      throw e
    }
    try {
      await context.queryClient.ensureQueryData(teamQuery(params.team))
    } catch (e) {
      if (e instanceof ApiError && (e.status === 403 || e.status === 404 || e.status === 400)) throw redirect({ to: "/" })
      throw e
    }
  },
  component: TeamLayout,
})

function TeamLayout() {
  const { team } = Route.useParams()
  return (
    <AppShell team={team}>
      <VerifyEmailBanner />
      <Outlet />
    </AppShell>
  )
}

function VerifyEmailBanner() {
  const { data: me } = useQuery(meQuery)
  const [hidden, setHidden] = useState(false)
  const resend = useApiMutation(() => post("/me/verify-email"), { success: "Lien de vérification envoyé" })
  if (!me || me.user.email_verified_at || hidden) return null
  return (
    <div className="mb-6 flex flex-col gap-3 rounded-lg border border-warning/40 bg-warning/10 px-4 py-3 text-sm sm:flex-row sm:items-center">
      <MailWarningIcon className="size-4 shrink-0 text-amber-600 dark:text-warning" />
      <p className="flex-1">Votre adresse {me.user.email} n'est pas encore vérifiée.</p>
      <div className="flex gap-2">
        <Button size="sm" variant="outline" loading={resend.isPending} onClick={() => resend.mutate(undefined)}>
          Renvoyer le lien
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setHidden(true)}>
          Plus tard
        </Button>
      </div>
    </div>
  )
}
