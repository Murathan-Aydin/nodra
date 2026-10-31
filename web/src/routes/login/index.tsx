import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { AlertCircleIcon } from "lucide-react"
import { AuthLayout } from "@/components/auth-layout"
import { Field } from "@/components/field"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { ApiError, post } from "@/lib/api"
import { instanceQuery, meQuery } from "@/lib/queries"
import type { User } from "@/lib/types"
import { OAuthButtons } from "@/features/auth/oauth-buttons"
import { safeRedirect } from "@/lib/utils"

interface LoginSearch {
  redirect?: string
  oauth_error?: string
  reset?: string
}

export const Route = createFileRoute("/login/")({
  validateSearch: (s: Record<string, unknown>): LoginSearch => ({
    redirect: typeof s.redirect === "string" ? s.redirect : undefined,
    oauth_error: typeof s.oauth_error === "string" ? s.oauth_error : undefined,
    reset: typeof s.reset === "string" ? s.reset : undefined,
  }),
  component: LoginPage,
})

function LoginPage() {
  const search = Route.useSearch()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data: instance } = useQuery(instanceQuery)
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState<string | undefined>(search.oauth_error)
  const [loading, setLoading] = useState(false)
  const target = safeRedirect(search.redirect)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError(undefined)
    try {
      const res = await post<{ mfa_required: boolean; user?: User }>("/auth/login", { email, password })
      if (res.mfa_required) {
        await navigate({ to: "/login/2fa", search: { redirect: target } })
        return
      }
      await qc.invalidateQueries({ queryKey: meQuery.queryKey })
      qc.removeQueries({ queryKey: ["team"] })
      await navigate({ to: target })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Connexion impossible")
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      title="Connexion"
      description="Accédez à vos serveurs et à vos déploiements."
      footer={
        instance?.registration_enabled && (
          <>
            Pas encore de compte ?{" "}
            <Link to="/register" search={{ redirect: search.redirect }} className="font-medium text-primary hover:underline">
              Créer un compte
            </Link>
          </>
        )
      }
    >
      <form onSubmit={submit} className="space-y-4">
        {search.reset && (
          <Alert variant="info">
            <AlertDescription>Mot de passe modifié : connectez-vous avec le nouveau.</AlertDescription>
          </Alert>
        )}
        {error && (
          <Alert variant="destructive">
            <AlertCircleIcon />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <Field label="Adresse e-mail" htmlFor="email">
          <Input id="email" type="email" autoComplete="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field
          label={
            <span className="flex w-full items-center justify-between">
              Mot de passe
              <Link to="/forgot-password" className="text-xs font-normal text-primary hover:underline">
                Mot de passe oublié ?
              </Link>
            </span>
          }
          htmlFor="password"
        >
          <Input id="password" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
        <Button type="submit" className="w-full" loading={loading}>
          Se connecter
        </Button>
        {!!instance?.oauth_providers?.length && (
          <>
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <Separator className="flex-1" /> ou <Separator className="flex-1" />
            </div>
            <OAuthButtons providers={instance.oauth_providers} redirect={target} />
          </>
        )}
      </form>
    </AuthLayout>
  )
}
