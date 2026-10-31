import { useMutation } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { Section } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { api } from "@/lib/api"
import { errorText } from "./dialogs"

/** Zone de danger : suppression confirmée en retapant le nom (demande d'approbation gérée). */
export function ConfirmDelete({ what, name, text, path, onDeleted }: { what: string; name: string; text: string; path: string; onDeleted: () => void }) {
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState("")
  const remove = useMutation({
    mutationFn: () => api<{ approval_request?: { required_approvals: number } } | undefined>(path, { method: "DELETE" }),
    onSuccess: (r) => {
      setOpen(false)
      if (r?.approval_request) {
        toast.info("Demande d'approbation envoyée", { description: `${r.approval_request.required_approvals} approbation(s) requise(s).` })
        return
      }
      toast.success(`${name} supprimé`)
      onDeleted()
    },
    onError: (e) => toast.error(errorText(e, "Suppression impossible")),
  })
  return (
    <Section className="border-destructive/40">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="font-semibold text-destructive">Zone de danger</h2>
          <p className="text-sm text-muted-foreground">{text}</p>
        </div>
        <Button variant="destructive" onClick={() => setOpen(true)}>
          Supprimer {what}
        </Button>
      </div>
      <Dialog
        open={open}
        onOpenChange={(o) => {
          setOpen(o)
          setTyped("")
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Supprimer {what} « {name} » ?
            </DialogTitle>
            <DialogDescription>Action définitive. {text}</DialogDescription>
          </DialogHeader>
          <Field label={`Tapez « ${name} » pour confirmer`} className="my-4">
            <Input autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} />
          </Field>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Annuler
            </Button>
            <Button variant="destructive" loading={remove.isPending} disabled={typed !== name} onClick={() => remove.mutate()}>
              Supprimer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Section>
  )
}
