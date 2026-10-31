import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { LogOutIcon, RocketIcon } from "lucide-react"
import { AuthPanel, AuthShell } from "@/components/auth-shell"
import { Button } from "@/components/ui/button"
import { post } from "@/lib/api"
import { meQuery } from "@/lib/queries"
import { requireSession } from "@/lib/session"

// Accueil provisoire : le tableau de bord sera reconstruit ensuite.
export const Route = createFileRoute("/")({
  beforeLoad: ({ context }) => requireSession(context.queryClient, "/"),
  component: Home,
})

function Home() {
  const me = useQuery(meQuery).data
  const navigate = useNavigate()
  const qc = useQueryClient()
  const logout = useMutation({
    mutationFn: () => post("/auth/logout"),
    onSuccess: () => {
      qc.clear()
      void navigate({ to: "/login" })
    },
  })
  return (
    <AuthShell>
      <AuthPanel title={`Bonjour ${me?.user.name || me?.user.email || ""}`} description="Vous êtes connecté. Le tableau de bord arrive à la prochaine étape.">
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => void navigate({ to: "/onboarding" })}>
            <RocketIcon /> Configuration guidée
          </Button>
          <Button variant="ghost" loading={logout.isPending} onClick={() => logout.mutate()}>
            <LogOutIcon /> Se déconnecter
          </Button>
        </div>
      </AuthPanel>
    </AuthShell>
  )
}
