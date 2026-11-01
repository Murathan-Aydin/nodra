import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { AuthPanel, AuthShell } from "@/components/auth-shell"
import { Field } from "@/components/field"
import { PasswordInput } from "@/components/password-input"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { post } from "@/lib/api"
import { requireSetupDone } from "@/lib/session"
import { safeRedirect } from "@/lib/utils"

export const Route = createFileRoute("/register")({
  validateSearch: (s: Record<string, unknown>): { redirect?: string } => (typeof s.redirect === "string" ? { redirect: s.redirect } : {}),
  beforeLoad: ({ context }) => requireSetupDone(context.queryClient),
  component: Register,
})

/** Inscription (si les inscriptions sont ouvertes, ou pour accepter une invitation). */
function Register() {
  const { redirect } = Route.useSearch()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [f, setF] = useState({ name: "", email: "", password: "" })
  const register = useMutation({
    mutationFn: () => post("/auth/register", { ...f, email: f.email.trim() }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["me"] })
      void navigate({ to: safeRedirect(redirect) })
    },
  })
  return (
    <AuthShell>
      <AuthPanel title="Créer un compte" description="Une équipe personnelle est créée pour vous.">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            register.mutate()
          }}
        >
          <Field label="Nom">
            <Input autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} autoComplete="name" />
          </Field>
          <Field label="Adresse e-mail">
            <Input required type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} autoComplete="email" />
          </Field>
          <Field label="Mot de passe" hint="8 caractères minimum.">
            <PasswordInput required minLength={8} value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />
          </Field>
          {register.error && <p className="text-sm text-destructive">{register.error.message}</p>}
          <Button type="submit" size="lg" className="w-full" loading={register.isPending}>
            Créer le compte
          </Button>
          <p className="text-center text-sm text-muted-foreground">
            Déjà un compte ?{" "}
            <Link to="/login" search={redirect ? { redirect } : {}} className="text-foreground hover:underline">
              Se connecter
            </Link>
          </p>
        </form>
      </AuthPanel>
    </AuthShell>
  )
}
