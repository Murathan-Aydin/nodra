import { createFileRoute, Link } from "@tanstack/react-router"
import { useState } from "react"
import { MailCheckIcon } from "lucide-react"
import { AuthLayout } from "@/components/auth-layout"
import { Field } from "@/components/field"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ApiError, post } from "@/lib/api"

export const Route = createFileRoute("/forgot-password")({ component: ForgotPage })

function ForgotPage() {
  const [email, setEmail] = useState("")
  const [sent, setSent] = useState(false)
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError(undefined)
    try {
      await post("/auth/password/forgot", { email })
      setSent(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Envoi impossible")
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      title="Mot de passe oublié"
      description="Recevez un lien pour choisir un nouveau mot de passe (valable 1 heure)."
      footer={
        <Link to="/login" className="font-medium text-primary hover:underline">
          Retour à la connexion
        </Link>
      }
    >
      {sent ? (
        <Alert variant="info">
          <MailCheckIcon />
          <AlertDescription>Si un compte existe pour {email}, un e-mail vient de lui être envoyé.</AlertDescription>
        </Alert>
      ) : (
        <form onSubmit={submit} className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <Field label="Adresse e-mail" htmlFor="email">
            <Input id="email" type="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
          </Field>
          <Button type="submit" className="w-full" loading={loading}>
            Envoyer le lien
          </Button>
        </form>
      )}
    </AuthLayout>
  )
}
