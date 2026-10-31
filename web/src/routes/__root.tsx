import { createRootRouteWithContext, Link, Outlet } from "@tanstack/react-router"
import type { QueryClient } from "@tanstack/react-query"
import { Button } from "@/components/ui/button"

export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: Outlet,
  notFoundComponent: NotFound,
  errorComponent: ({ error }) => (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 p-6 text-center">
      <p className="text-lg font-semibold">Une erreur est survenue</p>
      <p className="max-w-md text-sm text-muted-foreground">{error instanceof Error ? error.message : String(error)}</p>
      <Button onClick={() => location.reload()}>Recharger la page</Button>
    </div>
  ),
})

function NotFound() {
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-3 p-6 text-center">
      <p className="text-5xl font-semibold text-muted-foreground">404</p>
      <p className="text-sm text-muted-foreground">Cette page n'existe pas.</p>
      <Button asChild variant="outline">
        <Link to="/">Retour à l'accueil</Link>
      </Button>
    </div>
  )
}
