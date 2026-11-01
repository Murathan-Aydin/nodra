import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { AuthPanel, AuthShell } from "@/components/auth-shell"
import { Field } from "@/components/field"
import { PasswordInput } from "@/components/password-input"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ApiError, post } from "@/lib/api"
import { instanceQuery } from "@/lib/queries"
import { requireSetupDone } from "@/lib/session"
import { safeRedirect } from "@/lib/utils"

export const Route = createFileRoute("/login")({
  validateSearch: (s: Record<string, unknown>): { redirect?: string; oauth_error?: string } => ({
    ...(typeof s.redirect === "string" ? { redirect: s.redirect } : {}),
    ...(typeof s.oauth_error === "string" ? { oauth_error: s.oauth_error } : {}),
  }),
  beforeLoad: ({ context }) => requireSetupDone(context.queryClient),
  component: Login,
})

function Login() {
  const { redirect, oauth_error } = Route.useSearch()
  const instance = useQuery(instanceQuery).data
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [f, setF] = useState({ email: "", password: "" })
  const [mfa, setMfa] = useState(false)
  const [code, setCode] = useState("")
  const [recovery, setRecovery] = useState(false)
  const done = async () => {
    await qc.invalidateQueries({ queryKey: ["me"] })
    void navigate({ to: safeRedirect(redirect) })
  }
  const login = useMutation({
    mutationFn: () => post<{ mfa_required: boolean }>("/auth/login", { email: f.email.trim(), password: f.password }),
    onSuccess: (r) => (r.mfa_required ? setMfa(true) : void done()),
  })
  const verify = useMutation({
    mutationFn: () => post("/auth/login/2fa", recovery ? { recovery_code: code.trim() } : { code: code.replace(/\s/g, "") }),
    onSuccess: () => void done(),
  })
  const error = (login.error ?? verify.error) as Error | null

  return (
    <AuthShell>
      {mfa ? (
        <AuthPanel title="Double authentification" description={recovery ? "Saisissez un de vos codes de secours." : "Saisissez le code à 6 chiffres de votre application."}>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              verify.mutate()
            }}
          >
            <Field label={recovery ? "Code de secours" : "Code"}>
              <Input
                autoFocus
                required
                autoComplete="one-time-code"
                inputMode={recovery ? "text" : "numeric"}
                className="text-center font-mono text-lg tracking-[0.3em]"
                value={code}
                onChange={(e) => setCode(e.target.value)}
              />
            </Field>
            {error && <p className="text-sm text-destructive">{error instanceof ApiError ? error.message : "Vérification impossible"}</p>}
            <Button type="submit" size="lg" className="w-full" loading={verify.isPending}>
              Vérifier
            </Button>
            <div className="flex justify-between text-sm">
              <Button type="button" variant="link" className="h-auto p-0" onClick={() => (setRecovery(!recovery), setCode(""))}>
                {recovery ? "Utiliser l'application" : "Utiliser un code de secours"}
              </Button>
              <Button type="button" variant="link" className="h-auto p-0 text-muted-foreground" onClick={() => (setMfa(false), setCode(""))}>
                Retour
              </Button>
            </div>
          </form>
        </AuthPanel>
      ) : (
        <AuthPanel title="Connexion" description="Accédez à votre instance.">
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              login.mutate()
            }}
          >
            <Field label="Adresse e-mail">
              <Input required autoFocus type="email" autoComplete="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />
            </Field>
            <Field label="Mot de passe">
              <PasswordInput required autoComplete="current-password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} />
            </Field>
            <div className="-mt-2 text-right text-sm">
              <Link to="/forgot-password" className="text-muted-foreground hover:text-foreground">
                Mot de passe oublié ?
              </Link>
            </div>
            {error && <p className="text-sm text-destructive">{error instanceof ApiError ? error.message : "Connexion impossible"}</p>}
            {oauth_error && !error && <p className="text-sm text-destructive">{oauth_error}</p>}
            <Button type="submit" size="lg" className="w-full" loading={login.isPending}>
              Se connecter
            </Button>
          </form>
          {!!instance?.oauth_providers.length && (
            <div className="mt-5 space-y-2 border-t border-border pt-5">
              {instance.oauth_providers.map((p) => (
                <Button key={p.provider} variant="outline" className="w-full" asChild>
                  <a href={`/api/v1/auth/oauth/${p.provider}/start?redirect=${encodeURIComponent(safeRedirect(redirect))}`}>Continuer avec {p.name}</a>
                </Button>
              ))}
            </div>
          )}
          {instance?.registration_enabled && (
            <p className="mt-5 text-center text-sm text-muted-foreground">
              Pas de compte ?{" "}
              <Link to="/register" search={redirect ? { redirect } : {}} className="text-foreground hover:underline">
                Créer un compte
              </Link>
            </p>
          )}
        </AuthPanel>
      )}
    </AuthShell>
  )
}
