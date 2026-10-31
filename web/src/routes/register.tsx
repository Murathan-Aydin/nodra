import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { AlertCircleIcon } from "lucide-react"
import { AuthLayout } from "@/components/auth-layout"
import { Field } from "@/components/field"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ApiError, post } from "@/lib/api"
import { instanceQuery, meQuery } from "@/lib/queries"
import { safeRedirect } from "@/lib/utils"
import { OAuthButtons } from "@/features/auth/oauth-buttons"

export const Route = createFileRoute("/register")({
  validateSearch: (s: Record<string, unknown>): { redirect?: string } => (typeof s.redirect === "string" ? { redirect: s.redirect } : {}),
  component: RegisterPage,
})

function RegisterPage() {
  const { redirect } = Route.useSearch()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data: instance } = useQuery(instanceQuery)
  const [form, setForm] = useState({ name: "", email: "", password: "" })
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError(undefined)
    try {
      await post("/auth/register", form)
      await qc.invalidateQueries({ queryKey: meQuery.queryKey })
      await navigate({ to: safeRedirect(redirect) })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Inscription impossible")
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      title="Créer un compte"
      description="Le premier compte créé devient administrateur de l'instance."
      footer={
        <>
          Déjà un compte ?{" "}
          <Link to="/login" search={{ redirect }} className="font-medium text-primary hover:underline">
            Se connecter
          </Link>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-4">
        {instance && !instance.registration_enabled && (
          <Alert variant="warning">
            <AlertDescription>Les inscriptions sont fermées sur cette instance (sauf invitation).</AlertDescription>
          </Alert>
        )}
        {error && (
          <Alert variant="destructive">
            <AlertCircleIcon />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <Field label="Nom" htmlFor="name">
          <Input id="name" autoComplete="name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        </Field>
        <Field label="Adresse e-mail" htmlFor="email">
          <Input id="email" type="email" autoComplete="email" required value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
        </Field>
        <Field label="Mot de passe" htmlFor="password" hint="8 caractères minimum.">
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            required
            minLength={8}
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
          />
        </Field>
        <Button type="submit" className="w-full" loading={loading}>
          Créer mon compte
        </Button>
        {!!instance?.oauth_providers?.length && <OAuthButtons providers={instance.oauth_providers} redirect={safeRedirect(redirect)} />}
      </form>
    </AuthLayout>
  )
}
