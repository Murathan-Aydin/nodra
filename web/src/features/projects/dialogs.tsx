import { useNavigate } from "@tanstack/react-router"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { ApiError, post, teamPath } from "@/lib/api"
import type { Environment, ProjectSummary } from "@/lib/types"

export const errorText = (e: unknown, fallback: string) => (e instanceof ApiError ? e.message : fallback)

/** Formulaire nom + description dans une fenêtre (création de projet ou d'environnement). */
function NameDialog({
  open,
  onOpenChange,
  title,
  description,
  submit,
  pending,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  title: string
  description: string
  submit: string
  pending: boolean
  onSubmit: (v: { name: string; description: string }) => void
}) {
  const [f, setF] = useState({ name: "", description: "" })
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setF({ name: "", description: "" })
        onOpenChange(o)
      }}
    >
      <DialogContent>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            onSubmit({ name: f.name.trim(), description: f.description.trim() })
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <div className="my-5 space-y-4">
            <Field label="Nom">
              <Input required autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
            </Field>
            <Field label="Description">
              <Input value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
            </Field>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Annuler
            </Button>
            <Button type="submit" loading={pending} disabled={!f.name.trim()}>
              {submit}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function CreateProjectDialog({ team, open, onOpenChange }: { team: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const create = useMutation({
    mutationFn: (v: { name: string; description: string }) => post<ProjectSummary>(teamPath(team, "/projects"), v),
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: ["team", team, "projects"] })
      onOpenChange(false)
      void navigate({ to: "/t/$team/projects/$project", params: { team, project: p.id } })
    },
    onError: (e) => toast.error(errorText(e, "Création impossible")),
  })
  return (
    <NameDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Nouveau projet"
      description="Un environnement « production » est créé avec le projet."
      submit="Créer"
      pending={create.isPending}
      onSubmit={(v) => create.mutate(v)}
    />
  )
}

export function CreateEnvironmentDialog({ team, project, open, onOpenChange }: { team: string; project: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const create = useMutation({
    mutationFn: (v: { name: string; description: string }) => post<Environment>(teamPath(team, `/projects/${project}/environments`), v),
    onSuccess: (e) => {
      void qc.invalidateQueries({ queryKey: ["team", team, "project", project] })
      void qc.invalidateQueries({ queryKey: ["team", team, "projects"] })
      onOpenChange(false)
      void navigate({ to: "/t/$team/projects/$project/$env", params: { team, project, env: e.id } })
    },
    onError: (e) => toast.error(errorText(e, "Création impossible")),
  })
  return (
    <NameDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Nouvel environnement"
      description="Par exemple staging ou développement. Ses variables partagées s'appliquent à ses ressources."
      submit="Créer"
      pending={create.isPending}
      onSubmit={(v) => create.mutate(v)}
    />
  )
}
