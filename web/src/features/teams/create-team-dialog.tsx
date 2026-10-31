import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { teamsQuery } from "@/lib/queries"
import type { Team } from "@/lib/types"

/** Création d'une organisation, ou d'une sous-équipe de l'équipe courante. */
export function CreateTeamDialog({ open, onOpenChange, parentId }: { open: boolean; onOpenChange: (o: boolean) => void; parentId?: string }) {
  const navigate = useNavigate()
  const [name, setName] = useState("")
  const [asSub, setAsSub] = useState(false)
  const create = useApiMutation((v: { name: string; parent_id?: string }) => post<Team>("/teams", v), {
    success: "Équipe créée",
    invalidate: [teamsQuery.queryKey],
    onSuccess: (team) => {
      onOpenChange(false)
      setName("")
      void navigate({ to: "/t/$team", params: { team: team.id } })
    },
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Nouvelle équipe</DialogTitle>
          <DialogDescription>Vous en serez propriétaire. Les membres d'une organisation accèdent à ses sous-équipes.</DialogDescription>
        </DialogHeader>
        <form
          id="create-team"
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate({ name: name.trim(), parent_id: asSub ? parentId : undefined })
          }}
        >
          <Field label="Nom" htmlFor="team-name">
            <Input id="team-name" required autoFocus value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          {parentId && (
            <Label className="font-normal">
              <Checkbox checked={asSub} onCheckedChange={(c) => setAsSub(c === true)} /> Sous-équipe de l'équipe courante
            </Label>
          )}
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Annuler
          </Button>
          <Button type="submit" form="create-team" loading={create.isPending}>
            Créer
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
