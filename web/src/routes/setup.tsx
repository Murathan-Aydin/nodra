import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { InfoIcon, ShieldAlertIcon } from "lucide-react"
import { AuthPanel, AuthShell } from "@/components/auth-shell"
import { Field } from "@/components/field"
import { PasswordInput } from "@/components/password-input"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ApiError, post } from "@/lib/api"
import { instanceQuery } from "@/lib/queries"

// Premier démarrage : création du compte administrateur de l'instance.
export const Route = createFileRoute("/setup")({
  beforeLoad: async ({ context }) => {
    const instance = await context.queryClient.fetchQuery(instanceQuery)
    if (!instance.needs_setup) throw redirect({ to: "/login" })
  },
  component: Setup,
})

function Setup() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [f, setF] = useState({ name: "", email: "", password: "", confirm: "" })
  const mismatch = f.confirm !== "" && f.confirm !== f.password
  const create = useMutation({
    mutationFn: () => post("/auth/register", { name: f.name.trim(), email: f.email.trim(), password: f.password }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["instance"] })
      void navigate({ to: "/onboarding" })
    },
  })
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })
  return (
    <AuthShell>
      <AuthPanel title="Bienvenue" description="Créez le compte administrateur de cette instance.">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate()
          }}
        >
          <Alert variant="warning">
            <ShieldAlertIcon />
            <AlertTitle>Accès complet à l'instance</AlertTitle>
            <AlertDescription>Ce premier compte en devient l'administrateur : serveurs, équipes, réglages et mises à jour.</AlertDescription>
          </Alert>
          <Field label="Nom">
            <Input required autoFocus autoComplete="name" value={f.name} onChange={set("name")} />
          </Field>
          <Field label="Adresse e-mail">
            <Input required type="email" autoComplete="email" value={f.email} onChange={set("email")} />
          </Field>
          <Field label="Mot de passe">
            <PasswordInput required minLength={8} autoComplete="new-password" value={f.password} onChange={set("password")} />
          </Field>
          <Field label="Confirmation" error={mismatch ? "Les deux mots de passe diffèrent." : undefined}>
            <PasswordInput required autoComplete="new-password" value={f.confirm} onChange={set("confirm")} />
          </Field>
          <p className="flex items-start gap-2 rounded-lg bg-muted px-3 py-2.5 text-xs text-muted-foreground">
            <InfoIcon className="mt-px size-3.5 shrink-0" />8 caractères minimum ; une phrase de passe longue est le plus sûr.
          </p>
          {create.error && <p className="text-sm text-destructive">{create.error instanceof ApiError ? create.error.message : "Création impossible"}</p>}
          <Button type="submit" size="lg" className="w-full" loading={create.isPending} disabled={f.password.length < 8 || f.password !== f.confirm}>
            Créer le compte
          </Button>
        </form>
      </AuthPanel>
    </AuthShell>
  )
}
