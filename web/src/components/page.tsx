import { Link } from "@tanstack/react-router"
import type { ReactNode } from "react"
import { ChevronRightIcon } from "lucide-react"
import { cn } from "@/lib/utils"

export type Crumb = { label: string; to?: string }

/** En-tête de page : fil d'Ariane, titre, description et actions (comme les pages de Coolify). */
export function PageHeader({ crumbs, title, description, actions }: { crumbs?: Crumb[]; title: ReactNode; description?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="mb-6 space-y-2">
      {crumbs && crumbs.length > 0 && (
        <nav aria-label="Fil d'Ariane" className="flex flex-wrap items-center gap-1 text-sm text-muted-foreground">
          {crumbs.map((c, i) => (
            <span key={i} className="flex items-center gap-1">
              {i > 0 && <ChevronRightIcon className="size-3.5" />}
              {c.to ? (
                <Link to={c.to as "/"} className="hover:text-foreground">
                  {c.label}
                </Link>
              ) : (
                <span className="text-foreground">{c.label}</span>
              )}
            </span>
          ))}
        </nav>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="flex items-center gap-3 text-2xl font-semibold tracking-tight [text-shadow:0_1px_8px_rgb(0_0_0/0.25)]">{title}</h1>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
      {description && <p className="text-sm text-muted-foreground [text-shadow:0_1px_6px_rgb(0_0_0/0.2)]">{description}</p>}
    </header>
  )
}

/** Carte cliquable en verre (projets, environnements, ressources). */
export function CardLink({ to, children, className }: { to: string; children: ReactNode; className?: string }) {
  return (
    <Link to={to as "/"} className={cn("glass-panel group block rounded-2xl p-4 transition-all hover:-translate-y-0.5 hover:border-primary/50", className)}>
      {children}
    </Link>
  )
}

export function EmptyPanel({ icon, title, text, action }: { icon?: ReactNode; title: string; text?: ReactNode; action?: ReactNode }) {
  return (
    <div className="glass-panel flex flex-col items-center gap-3 rounded-2xl px-6 py-12 text-center">
      {icon && <span className="text-muted-foreground [&_svg]:size-8">{icon}</span>}
      <p className="font-medium">{title}</p>
      {text && <p className="max-w-md text-sm text-muted-foreground">{text}</p>}
      {action}
    </div>
  )
}

/** Panneau de formulaire / réglages. */
export function Section({ title, description, children, className }: { title?: string; description?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn("glass-panel rounded-2xl p-6", className)}>
      {(title || description) && (
        <header className="mb-5 space-y-1">
          {title && <h2 className="font-semibold">{title}</h2>}
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </header>
      )}
      {children}
    </section>
  )
}
