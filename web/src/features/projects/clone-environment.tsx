import { useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { post, teamPath } from "@/lib/api"
import { teamQueries } from "@/lib/queries"
import type { Environment } from "@/lib/types"
import { errorText } from "./dialogs"

/** Clonage d'un environnement (avec ou sans ses ressources), vers ce projet ou un autre. */
export function CloneEnvironmentDialog({
  team,
  project,
  environment,
  open,
  onOpenChange,
}: {
  team: string
  project: string
  environment: Environment
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const projects = useQuery({ ...teamQueries.projects(team), enabled: open }).data ?? []
  const [f, setF] = useState({ name: `${environment.name}-copie`, project_id: project, with_resources: false })
  const clone = useMutation({
    mutationFn: () => post<Environment>(teamPath(team, `/projects/${project}/environments/${environment.id}/clone`), f),
    onSuccess: (e) => {
      void qc.invalidateQueries({ queryKey: ["team", team] })
      onOpenChange(false)
      toast.success("Environnement cloné")
      void navigate({ to: "/t/$team/projects/$project/$env", params: { team, project: f.project_id, env: e.id } })
    },
    onError: (e) => toast.error(errorText(e, "Clonage impossible")),
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Cloner « {environment.name} »</DialogTitle>
          <DialogDescription>Copie l'environnement et ses variables partagées ; les ressources copiées sont créées arrêtées, avec de nouveaux domaines.</DialogDescription>
        </DialogHeader>
        <div className="my-4 space-y-4">
          <Field label="Nom du nouvel environnement">
            <Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
          </Field>
          <Field label="Projet">
            <Select value={f.project_id} onValueChange={(v) => setF({ ...f, project_id: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {projects.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Label className="font-normal">
            <Checkbox checked={f.with_resources} onCheckedChange={(c) => setF({ ...f, with_resources: c === true })} /> Cloner aussi les ressources (sans leurs données)
          </Label>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Annuler
          </Button>
          <Button loading={clone.isPending} disabled={!f.name.trim()} onClick={() => clone.mutate()}>
            Cloner
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
