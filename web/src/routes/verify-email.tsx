import { createFileRoute, Link } from "@tanstack/react-router"
import { useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef, useState } from "react"
import { CheckCircle2Icon } from "lucide-react"
import { AuthLayout } from "@/components/auth-layout"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { ApiError, post } from "@/lib/api"
import { meQuery } from "@/lib/queries"

export const Route = createFileRoute("/verify-email")({
  validateSearch: (s: Record<string, unknown>) => ({ token: typeof s.token === "string" ? s.token : "" }),
  component: VerifyPage,
})

function VerifyPage() {
  const { token } = Route.useSearch()
  const qc = useQueryClient()
  const [state, setState] = useState<"pending" | "ok" | "error">("pending")
  const [error, setError] = useState<string>()
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return // une seule vérification (jeton à usage unique)
    started.current = true
    post("/auth/verify-email", { token })
      .then(() => {
        setState("ok")
        void qc.invalidateQueries({ queryKey: meQuery.queryKey })
      })
      .catch((err) => {
        setState("error")
        setError(err instanceof ApiError ? err.message : "Vérification impossible")
      })
  }, [token, qc])

  return (
    <AuthLayout title="Vérification de l'adresse">
      {state === "pending" && <p className="text-sm text-muted-foreground">Vérification en cours…</p>}
      {state === "ok" && (
        <div className="space-y-4">
          <Alert variant="info">
            <CheckCircle2Icon />
            <AlertDescription>Votre adresse e-mail est vérifiée.</AlertDescription>
          </Alert>
          <Button asChild className="w-full">
            <Link to="/">Continuer</Link>
          </Button>
        </div>
      )}
      {state === "error" && (
        <Alert variant="destructive">
          <AlertDescription>{error} Vous pouvez demander un nouveau lien depuis votre compte.</AlertDescription>
        </Alert>
      )}
    </AuthLayout>
  )
}
