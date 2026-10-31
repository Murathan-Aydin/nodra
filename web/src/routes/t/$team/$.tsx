import { createFileRoute, Link } from "@tanstack/react-router"
import { HammerIcon } from "lucide-react"
import { Button } from "@/components/ui/button"

// Pages pas encore reconstruites : la navigation reste utilisable.
export const Route = createFileRoute("/t/$team/$")({ component: UnderConstruction })

function UnderConstruction() {
  const { team } = Route.useParams()
  return (
    <div className="glass-panel mx-auto mt-16 flex max-w-md flex-col items-center gap-3 rounded-2xl px-8 py-12 text-center">
      <HammerIcon className="size-8 text-muted-foreground" />
      <h1 className="text-lg font-semibold">Page en construction</h1>
      <p className="text-sm text-muted-foreground">Cette partie de Nodra est en cours de reconstruction.</p>
      <Button variant="outline" asChild>
        <Link to="/t/$team" params={{ team }}>
          Retour au tableau de bord
        </Link>
      </Button>
    </div>
  )
}
