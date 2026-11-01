import { Link } from "@tanstack/react-router"
import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

export type Crumb = { label: string; to?: string }

/**
 * En-tête de page : titre, description et actions (comme les pages de Coolify).
 * Le fil d'Ariane est dans la barre du haut (components/top-breadcrumb.tsx), déduit de l'URL.
 */
export function PageHeader({ title, description, actions }: { /** @deprecated ignoré : le fil d'Ariane est dans la barre du haut. */ crumbs?: Crumb[]; title?: ReactNode; description?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="mb-6 space-y-2">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="flex items-center gap-3 text-2xl font-semibold tracking-tight [text-shadow:0_1px_8px_rgb(0_0_0/0.25)]">{title}</h1>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
      {description && <p className="text-sm font-medium text-foreground/85 [text-shadow:0_1px_2px_rgb(0_0_0/0.6),0_1px_10px_rgb(0_0_0/0.5)]">{description}</p>}
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

/**
 * Carte de réglages comme dans Coolify : titre souligné et actions sur la boîte extérieure,
 * contenu dans un « puits » intérieur. `bodyClassName` règle l'espacement du puits (p-5 par défaut).
 */
export function SettingsCard({
  title,
  description,
  actions,
  children,
  bodyClassName,
  className,
}: {
  title: string
  description?: ReactNode
  actions?: ReactNode
  children: ReactNode
  bodyClassName?: string
  className?: string
}) {
  return (
    <section className={cn("settings-card glass-panel", className)}>
      <header className="settings-card-header">
        <div>
          <h2>{title}</h2>
          {description && <p>{description}</p>}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </header>
      <div className={cn("settings-card-body", bodyClassName ?? "p-5")}>{children}</div>
    </section>
  )
}
