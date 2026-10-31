import { Children, cloneElement, isValidElement, useId, type ReactNode } from "react"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

/**
 * Champ de formulaire : libellé, contrôle, aide ou erreur.
 * Sans `htmlFor`, un contrôle unique reçoit un identifiant généré pour que le libellé lui soit associé.
 */
export function Field({
  label,
  htmlFor,
  hint,
  error,
  children,
  className,
}: {
  label: ReactNode
  htmlFor?: string
  hint?: ReactNode
  error?: string
  children: ReactNode
  className?: string
}) {
  const auto = useId()
  const only = Children.count(children) === 1 && isValidElement<{ id?: string }>(children) ? children : null
  const id = htmlFor ?? only?.props.id ?? (only ? auto : undefined)
  return (
    <div className={cn("space-y-1.5", className)}>
      <Label htmlFor={id}>{label}</Label>
      {only && !htmlFor && !only.props.id ? cloneElement(only, { id }) : children}
      {error ? <p className="text-xs text-destructive">{error}</p> : hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}
