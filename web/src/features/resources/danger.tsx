import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { api } from "@/lib/api"
import { resourceApi, type ResourceRef } from "./resource"

/** Onglet « Zone de danger » : suppression de la ressource (soumise aux approbations). */
export function DangerTab({ resource, name, back }: { resource: ResourceRef; name: string; back: string }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [volumes, setVolumes] = useState(false)
  const remove = useMutation({
    mutationFn: () => api<{ approval_request?: unknown } | undefined>(resourceApi(resource, volumes ? "?delete_volumes=true" : ""), { method: "DELETE" }),
    onSuccess: (r) => {
      setOpen(false)
      if (r && "approval_request" in r && r.approval_request) {
        toast.info("Suppression en attente d'approbation")
        return
      }
      toast.success(`« ${name} » supprimée`)
      void qc.invalidateQueries({ queryKey: ["team", resource.team] })
      void navigate({ to: back })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Zone de danger">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="text-sm font-semibold">Supprimer la ressource</p>
          <p className="text-sm text-muted-foreground">Arrête les conteneurs puis supprime la configuration, les variables, les stockages et les tags. Action irréversible.</p>
        </div>
        <Button variant="destructive" size="sm" onClick={() => setOpen(true)}>
          Supprimer
        </Button>
      </div>
      <ConfirmDialog open={open} onOpenChange={setOpen} title={`Supprimer « ${name} » ?`} confirmText={name} actionLabel="Supprimer définitivement" loading={remove.isPending} onConfirm={() => remove.mutate()}>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={volumes} onCheckedChange={(c) => setVolumes(c === true)} />
          Supprimer aussi les volumes Docker (données perdues)
        </label>
      </ConfirmDialog>
    </SettingsCard>
  )
}
