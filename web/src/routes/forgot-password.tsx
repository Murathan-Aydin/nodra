import { createFileRoute, Link } from "@tanstack/react-router"
import { useMutation } from "@tanstack/react-query"
import { useState } from "react"
import { AuthPanel, AuthShell } from "@/components/auth-shell"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { post } from "@/lib/api"

export const Route = createFileRoute("/forgot-password")({ component: Forgot })

/** Mot de passe oublié : envoie un lien de réinitialisation (valable 1 h). */
function Forgot() {
  const [email, setEmail] = useState("")
  const send = useMutation({ mutationFn: () => post("/auth/password/forgot", { email: email.trim() }) })
  return (
    <AuthShell>
      <AuthPanel title="Mot de passe oublié" description="Recevez un lien pour choisir un nouveau mot de passe.">
        {send.isSuccess ? (
          <p className="text-sm">Si un compte existe pour {email}, un e-mail vient d'être envoyé. Le lien est valable une heure.</p>
        ) : (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              send.mutate()
            }}
          >
            <Field label="Adresse e-mail">
              <Input required autoFocus type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
            </Field>
            <Button type="submit" size="lg" className="w-full" loading={send.isPending}>
              Envoyer le lien
            </Button>
          </form>
        )}
        <p className="mt-5 text-center text-sm">
          <Link to="/login" className="text-muted-foreground hover:text-foreground">
            Retour à la connexion
          </Link>
        </p>
      </AuthPanel>
    </AuthShell>
  )
}
