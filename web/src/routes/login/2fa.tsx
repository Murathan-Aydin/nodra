import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { AlertCircleIcon } from "lucide-react"
import { AuthLayout } from "@/components/auth-layout"
import { Field } from "@/components/field"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ApiError, post } from "@/lib/api"
import { meQuery } from "@/lib/queries"
import { safeRedirect } from "@/lib/utils"

export const Route = createFileRoute("/login/2fa")({
  validateSearch: (s: Record<string, unknown>) => ({ redirect: typeof s.redirect === "string" ? s.redirect : undefined }),
  component: TwoFactorPage,
})

function TwoFactorPage() {
  const { redirect } = Route.useSearch()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [recovery, setRecovery] = useState(false)
  const [code, setCode] = useState("")
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError(undefined)
    try {
      await post("/auth/login/2fa", recovery ? { recovery_code: code.trim() } : { code: code.replace(/\s/g, "") })
      await qc.invalidateQueries({ queryKey: meQuery.queryKey })
      await navigate({ to: safeRedirect(redirect) })
    } catch (err) {
      if (err instanceof ApiError && err.message.includes("aucune connexion")) {
        await navigate({ to: "/login", search: { redirect } })
        return
      }
      setError(err instanceof ApiError ? err.message : "Vérification impossible")
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      title="Double authentification"
      description={recovery ? "Saisissez l'un de vos codes de secours (utilisable une seule fois)." : "Saisissez le code à 6 chiffres de votre application d'authentification."}
    >
      <form onSubmit={submit} className="space-y-4">
        {error && (
          <Alert variant="destructive">
            <AlertCircleIcon />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <Field label={recovery ? "Code de secours" : "Code"} htmlFor="code">
          <Input
            id="code"
            autoFocus
            required
            autoComplete="one-time-code"
            inputMode={recovery ? "text" : "numeric"}
            className={recovery ? "font-mono" : "text-center font-mono text-lg tracking-[0.4em]"}
            maxLength={recovery ? 32 : 7}
            value={code}
            onChange={(e) => setCode(e.target.value)}
          />
        </Field>
        <Button type="submit" className="w-full" loading={loading}>
          Vérifier
        </Button>
        <Button
          type="button"
          variant="link"
          className="w-full"
          onClick={() => {
            setRecovery((r) => !r)
            setCode("")
            setError(undefined)
          }}
        >
          {recovery ? "Utiliser un code de l'application" : "Utiliser un code de secours"}
        </Button>
      </form>
    </AuthLayout>
  )
}
