import { createFileRoute, Link } from "@tanstack/react-router"
import { useMutation } from "@tanstack/react-query"
import { useEffect } from "react"
import { AuthPanel, AuthShell } from "@/components/auth-shell"
import { Button } from "@/components/ui/button"
import { post } from "@/lib/api"

export const Route = createFileRoute("/verify-email")({
  validateSearch: (s: Record<string, unknown>): { token?: string } => (typeof s.token === "string" ? { token: s.token } : {}),
  component: Verify,
})

/** Vérification de l'adresse e-mail depuis le lien reçu (valable 24 h). */
function Verify() {
  const { token } = Route.useSearch()
  const verify = useMutation({ mutationFn: () => post("/auth/verify-email", { token }) })
  const { mutate } = verify
  useEffect(() => {
    if (token) mutate()
  }, [token, mutate])
  return (
    <AuthShell>
      <AuthPanel title="Vérification de l'adresse">
        <p className="mb-4 text-sm">
          {!token ? "Lien incomplet : jeton manquant." : verify.isPending ? "Vérification…" : verify.isSuccess ? "Adresse vérifiée, merci." : verify.error ? verify.error.message : ""}
        </p>
        <Button className="w-full" asChild>
          <Link to="/">Continuer</Link>
        </Button>
      </AuthPanel>
    </AuthShell>
  )
}
