import { useQuery } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import { PaletteIcon } from "lucide-react"
import { AppearanceDialog } from "@/components/appearance-dialog"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"
import { instanceQuery } from "@/lib/queries"
import { cn } from "@/lib/utils"

/**
 * Écrans hors application (premier démarrage, connexion, configuration guidée) :
 * fond d'écran, marque de l'instance, contenu centré et accès au réglage d'apparence.
 */
export function AuthShell({ children, wide, className }: { children: ReactNode; wide?: boolean; className?: string }) {
  const instance = useQuery(instanceQuery).data
  const [appearance, setAppearance] = useState(false)
  return (
    <div className="relative flex min-h-screen flex-col items-center px-4 py-10">
      <Button variant="ghost" size="sm" className="absolute top-4 right-4" onClick={() => setAppearance(true)}>
        <PaletteIcon /> Apparence
      </Button>
      <div className="mb-8 flex items-center gap-3">
        <Logo className="size-10 rounded-xl shadow-lg" />
        <span className="text-2xl font-bold tracking-tight">{instance?.name ?? "Nodra"}</span>
      </div>
      <main className={cn("flex w-full flex-1 flex-col items-center", wide ? "max-w-3xl" : "max-w-md justify-center pb-24", className)}>{children}</main>
      {instance?.version && <p className="mt-8 font-mono text-[11px] text-muted-foreground/60">Nodra {instance.version}</p>}
      <AppearanceDialog open={appearance} onOpenChange={setAppearance} />
    </div>
  )
}

/** Panneau principal en verre des écrans publics. */
export function AuthPanel({ title, description, children, className }: { title: ReactNode; description?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn("glass-panel-strong w-full rounded-[var(--radius-sheet)] p-7", className)}>
      <header className="mb-6 space-y-1.5">
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </header>
      {children}
    </section>
  )
}
