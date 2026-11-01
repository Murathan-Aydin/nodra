import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { api, get, post } from "@/lib/api"
import { teamQueries } from "@/lib/queries"
import type { Environment, ProjectSummary } from "@/lib/types"
import { resourceApi, resourcePage, type ResourceRef } from "./resource"

/** Environnements de l'équipe, groupés par projet (pour cloner ou déplacer). */
function useEnvironments(team: string) {
  const projects = useQuery(teamQueries.projects(team)).data ?? []
  const details = useQuery({
    queryKey: ["team", team, "projects", "environments"],
    queryFn: () => Promise.all(projects.map((p) => get<{ project: ProjectSummary; environments: Environment[] }>(`/teams/${team}/projects/${p.id}`))),
    enabled: projects.length > 0,
  })
  return details.data ?? []
}

function EnvironmentSelect({ team, value, onChange }: { team: string; value: string; onChange: (v: string) => void }) {
  const groups = useEnvironments(team)
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger>
        <SelectValue placeholder="Choisir un environnement" />
      </SelectTrigger>
      <SelectContent>
        {groups.map((g) => (
          <SelectGroup key={g.project.id}>
            <SelectLabel>{g.project.name}</SelectLabel>
            {g.environments.map((e) => (
              <SelectItem key={e.id} value={e.id}>
                {e.name}
              </SelectItem>
            ))}
          </SelectGroup>
        ))}
      </SelectContent>
    </Select>
  )
}

/** Onglet « Opérations » (Resource Operations du fork) : cloner et déplacer la ressource. */
export function OperationsTab({ resource, name, environmentId }: { resource: ResourceRef; name: string; environmentId: string }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [cloneName, setCloneName] = useState(`${name} (copie)`)
  const [cloneEnv, setCloneEnv] = useState(environmentId)
  const [moveEnv, setMoveEnv] = useState("")
  const [confirmMove, setConfirmMove] = useState(false)
  const clone = useMutation({
    mutationFn: () => post<{ id: string }>(resourceApi(resource, "/clone"), { name: cloneName, environment_id: cloneEnv }),
    onSuccess: (r) => {
      toast.success("Ressource clonée", { description: "Copie arrêtée, avec de nouveaux domaines. Les données des volumes ne sont pas copiées." })
      void qc.invalidateQueries({ queryKey: ["team", resource.team, "resources"] })
      void navigate({ to: resourcePage({ ...resource, id: r.id }) })
    },
    onError: (e) => toast.error(e.message),
  })
  const move = useMutation({
    mutationFn: () => api(resourceApi(resource, "/move"), { method: "POST", json: { environment_id: moveEnv } }),
    onSuccess: () => {
      toast.success("Ressource déplacée")
      setConfirmMove(false)
      void qc.invalidateQueries({ queryKey: ["team", resource.team] })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Cloner la ressource"
        description="Copie la configuration, les variables, les stockages (volumes renommés, données non copiées), les tags et les tâches planifiées (désactivées)."
        actions={
          <Button size="sm" variant="outline" disabled={!cloneName.trim() || !cloneEnv} loading={clone.isPending} onClick={() => clone.mutate()}>
            Cloner
          </Button>
        }
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Nom de la copie">
            <Input value={cloneName} onChange={(e) => setCloneName(e.target.value)} />
          </Field>
          <Field label="Environnement">
            <EnvironmentSelect team={resource.team} value={cloneEnv} onChange={setCloneEnv} />
          </Field>
        </div>
      </SettingsCard>
      <SettingsCard
        title="Déplacer la ressource"
        description="Change de projet ou d'environnement. Le serveur et les conteneurs ne changent pas ; les variables partagées du nouvel environnement s'appliquent au prochain déploiement."
        actions={
          <Button size="sm" variant="outline" disabled={!moveEnv || moveEnv === environmentId} onClick={() => setConfirmMove(true)}>
            Déplacer
          </Button>
        }
      >
        <Field label="Nouvel environnement">
          <EnvironmentSelect team={resource.team} value={moveEnv} onChange={setMoveEnv} />
        </Field>
      </SettingsCard>
      <ConfirmDialog
        open={confirmMove}
        onOpenChange={setConfirmMove}
        title={`Déplacer « ${name} » ?`}
        destructive={false}
        actionLabel="Déplacer"
        loading={move.isPending}
        onConfirm={() => move.mutate()}
      />
    </div>
  )
}

/** Variante de Switch avec libellé, utilisée dans plusieurs onglets. */
export function SwitchRow({ label, hint, checked, onChange, disabled }: { label: string; hint?: string; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <label className="flex items-start justify-between gap-4 py-2">
      <span>
        <span className="block text-sm font-medium">{label}</span>
        {hint && <span className="block text-xs text-muted-foreground">{hint}</span>}
      </span>
      <Switch checked={checked} onCheckedChange={onChange} disabled={disabled} />
    </label>
  )
}
