import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { AuthLayout } from "@/components/auth-layout"
import { Field } from "@/components/field"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ApiError, post } from "@/lib/api"

export const Route = createFileRoute("/reset-password")({
  validateSearch: (s: Record<string, unknown>) => ({ token: typeof s.token === "string" ? s.token : "" }),
  component: ResetPage,
})

function ResetPage() {
  const { token } = Route.useSearch()
  const navigate = useNavigate()
  const [password, setPassword] = useState("")
  const [confirm, setConfirm] = useState("")
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (password !== confirm) {
      setError("Les deux mots de passe ne correspondent pas.")
      return
    }
    setLoading(true)
    setError(undefined)
    try {
      await post("/auth/password/reset", { token, password })
      await navigate({ to: "/login", search: { reset: "1" } })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Réinitialisation impossible")
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout title="Nouveau mot de passe" description="Toutes vos sessions ouvertes seront fermées.">
      {!token ? (
        <Alert variant="destructive">
          <AlertDescription>
            Lien incomplet. <Link to="/forgot-password" className="underline">Demandez un nouveau lien</Link>.
          </AlertDescription>
        </Alert>
      ) : (
        <form onSubmit={submit} className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <Field label="Nouveau mot de passe" htmlFor="password" hint="8 caractères minimum.">
            <Input id="password" type="password" autoComplete="new-password" required minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} />
          </Field>
          <Field label="Confirmation" htmlFor="confirm">
            <Input id="confirm" type="password" autoComplete="new-password" required value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          </Field>
          <Button type="submit" className="w-full" loading={loading}>
            Enregistrer
          </Button>
        </form>
      )}
    </AuthLayout>
  )
}
