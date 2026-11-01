import { createFileRoute, Link } from "@tanstack/react-router"
import { useMutation } from "@tanstack/react-query"
import { useState } from "react"
import { AuthPanel, AuthShell } from "@/components/auth-shell"
import { Field } from "@/components/field"
import { PasswordInput } from "@/components/password-input"
import { Button } from "@/components/ui/button"
import { post } from "@/lib/api"

export const Route = createFileRoute("/reset-password")({
  validateSearch: (s: Record<string, unknown>): { token?: string } => (typeof s.token === "string" ? { token: s.token } : {}),
  component: Reset,
})

/** Nouveau mot de passe depuis le lien reçu par e-mail ; toutes les sessions sont fermées. */
function Reset() {
  const { token } = Route.useSearch()
  const [password, setPassword] = useState("")
  const reset = useMutation({ mutationFn: () => post("/auth/password/reset", { token, password }) })
  return (
    <AuthShell>
      <AuthPanel title="Nouveau mot de passe">
        {reset.isSuccess ? (
          <div className="space-y-4">
            <p className="text-sm">Mot de passe changé. Toutes vos sessions ont été fermées.</p>
            <Button className="w-full" asChild>
              <Link to="/login">Se connecter</Link>
            </Button>
          </div>
        ) : (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              reset.mutate()
            }}
          >
            <Field label="Nouveau mot de passe" hint="8 caractères minimum.">
              <PasswordInput required autoFocus minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
            </Field>
            {!token && <p className="text-sm text-destructive">Lien incomplet : jeton manquant.</p>}
            {reset.error && <p className="text-sm text-destructive">{reset.error.message}</p>}
            <Button type="submit" size="lg" className="w-full" disabled={!token} loading={reset.isPending}>
              Enregistrer
            </Button>
          </form>
        )}
      </AuthPanel>
    </AuthShell>
  )
}
