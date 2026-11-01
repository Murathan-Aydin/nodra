import { useState, type ReactNode } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"

/**
 * Confirmation d'une action (destructive le plus souvent) ; avec `confirmText`,
 * l'utilisateur doit recopier ce texte (le nom de la ressource) pour valider.
 */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmText,
  actionLabel = "Confirmer",
  destructive = true,
  loading,
  onConfirm,
  children,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: ReactNode
  confirmText?: string
  actionLabel?: string
  destructive?: boolean
  loading?: boolean
  onConfirm: () => void
  children?: ReactNode
}) {
  const [typed, setTyped] = useState("")
  const ok = !confirmText || typed === confirmText
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setTyped("")
        onOpenChange(o)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {children}
        {confirmText && (
          <div className="space-y-1.5">
            <p className="text-sm text-muted-foreground">
              Tapez <strong className="font-mono text-foreground">{confirmText}</strong> pour confirmer.
            </p>
            <Input value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus />
          </div>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Annuler
          </Button>
          <Button variant={destructive ? "destructive" : "default"} disabled={!ok} loading={loading} onClick={onConfirm}>
            {actionLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
