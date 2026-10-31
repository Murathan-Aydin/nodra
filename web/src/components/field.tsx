import { Children, cloneElement, isValidElement, useId, type ReactNode } from "react"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

/** Libellé, contrôle (associé automatiquement au libellé) et aide ou erreur. */
export function Field({ label, hint, error, children, className }: { label: ReactNode; hint?: ReactNode; error?: string; children: ReactNode; className?: string }) {
  const id = useId()
  const only = Children.count(children) === 1 && isValidElement<{ id?: string }>(children) ? children : null
  return (
    <div className={cn("space-y-1.5", className)}>
      <Label htmlFor={only?.props.id ?? id}>{label}</Label>
      {only && !only.props.id ? cloneElement(only, { id }) : children}
      {error ? <p className="text-xs text-destructive">{error}</p> : hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}
