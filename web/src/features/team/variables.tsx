import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { EyeIcon, EyeOffIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { PageHeader, SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import { api, get, post } from "@/lib/api"
import { teamQueries } from "@/lib/queries"
import type { Environment, ProjectSummary } from "@/lib/types"

type Scope = { type: "team" | "project" | "environment" | "server"; id: string }
type SharedVar = { id: string; key: string; value: string; comment: string; is_secret: boolean; is_multiline: boolean; is_literal: boolean }

const REF: Record<Scope["type"], string> = { team: "team", project: "project", environment: "environment", server: "server" }

/** Page « Variables partagées » (Shared Variables du fork) : par équipe, projet, environnement ou serveur. */
export function SharedVariablesPage({ team }: { team: string }) {
  const projects = useQuery(teamQueries.projects(team)).data ?? []
  const servers = useQuery(teamQueries.servers(team)).data ?? []
  const envs = useQuery({
    queryKey: ["team", team, "projects", "environments"],
    queryFn: () => Promise.all(projects.map((p) => get<{ project: ProjectSummary; environments: Environment[] }>(`/teams/${team}/projects/${p.id}`))),
    enabled: projects.length > 0,
  }).data
  const [scope, setScope] = useState<Scope>({ type: "team", id: team })
  const value = `${scope.type}:${scope.id}`
  return (
    <>
      <PageHeader title="Variables partagées" description="Réutilisables dans les variables des ressources : {{team.CLÉ}}, {{project.CLÉ}}, {{environment.CLÉ}}, {{server.CLÉ}}." />
      <div className="mb-6 max-w-sm">
        <Select
          value={value}
          onValueChange={(v) => {
            const [type, id] = v.split(":")
            setScope({ type: type as Scope["type"], id })
          }}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={`team:${team}`}>Équipe</SelectItem>
            <SelectGroup>
              <SelectLabel>Projets</SelectLabel>
              {projects.map((p) => (
                <SelectItem key={p.id} value={`project:${p.id}`}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectGroup>
            {envs?.map((g) => (
              <SelectGroup key={g.project.id}>
                <SelectLabel>Environnements · {g.project.name}</SelectLabel>
                {g.environments.map((e) => (
                  <SelectItem key={e.id} value={`environment:${e.id}`}>
                    {g.project.name} / {e.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            ))}
            <SelectGroup>
              <SelectLabel>Serveurs</SelectLabel>
              {servers.map((s) => (
                <SelectItem key={s.id} value={`server:${s.id}`}>
                  {s.name}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <VariablesList key={value} team={team} scope={scope} />
    </>
  )
}

function VariablesList({ team, scope }: { team: string; scope: Scope }) {
  const qc = useQueryClient()
  const key = ["team", team, "variables", scope.type, scope.id]
  const vars = useQuery({ queryKey: key, queryFn: () => get<SharedVar[]>(`/teams/${team}/variables?scope_type=${scope.type}&scope_id=${scope.id}`) })
  const [d, setD] = useState({ key: "", value: "", is_secret: false })
  const add = useMutation({
    mutationFn: () => post(`/teams/${team}/variables`, { scope, ...d }),
    onSuccess: () => {
      setD({ key: "", value: "", is_secret: false })
      void qc.invalidateQueries({ queryKey: key })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard title="Variables" description={`Utilisation : {{${REF[scope.type]}.CLÉ}}`}>
      <div className="space-y-2">
        {vars.data?.map((v) => <VarRow key={v.id + v.value} team={team} v={v} queryKey={key} />)}
        {vars.data?.length === 0 && <p className="py-2 text-sm text-muted-foreground">Aucune variable sur cette portée.</p>}
        <form
          className="grid gap-2 rounded-xl border border-dashed border-border p-3 md:grid-cols-[minmax(10rem,18rem)_1fr_auto_auto] md:items-center"
          onSubmit={(e) => {
            e.preventDefault()
            if (d.key) add.mutate()
          }}
        >
          <Input value={d.key} onChange={(e) => setD({ ...d, key: e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "_") })} placeholder="CLÉ" className="font-mono" />
          <Input value={d.value} onChange={(e) => setD({ ...d, value: e.target.value })} placeholder="valeur" className="font-mono" />
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <Checkbox checked={d.is_secret} onCheckedChange={(c) => setD({ ...d, is_secret: c === true })} /> Secret
          </label>
          <Button size="sm" type="submit" disabled={!d.key} loading={add.isPending}>
            <PlusIcon /> Ajouter
          </Button>
        </form>
      </div>
    </SettingsCard>
  )
}

function VarRow({ team, v, queryKey }: { team: string; v: SharedVar; queryKey: unknown[] }) {
  const qc = useQueryClient()
  const [value, setValue] = useState(v.value)
  const [shown, setShown] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const save = useMutation({
    mutationFn: () => api(`/teams/${team}/variables/${v.id}`, { method: "PATCH", json: { value } }),
    onSuccess: () => (toast.success(`${v.key} enregistrée`), void qc.invalidateQueries({ queryKey })),
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api(`/teams/${team}/variables/${v.id}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey }),
    onError: (e) => toast.error(e.message),
  })
  return (
    <div className="grid gap-2 rounded-xl border border-border p-3 md:grid-cols-[minmax(10rem,18rem)_1fr_auto]">
      <Input value={v.key} readOnly className="font-mono" />
      <div className="relative">
        <Input value={value} onChange={(e) => setValue(e.target.value)} type={v.is_secret && !shown ? "password" : "text"} placeholder={v.is_secret ? "Secret : saisir une nouvelle valeur" : undefined} className="pr-9 font-mono" />
        {v.is_secret && (
          <button type="button" onClick={() => setShown(!shown)} className="absolute top-2 right-2 text-muted-foreground hover:text-foreground" aria-label={shown ? "Masquer" : "Afficher"}>
            {shown ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
          </button>
        )}
      </div>
      <div className="flex gap-1">
        <Button size="sm" variant="outline" disabled={value === v.value} loading={save.isPending} onClick={() => save.mutate()}>
          Enregistrer
        </Button>
        <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
          <Trash2Icon />
        </Button>
      </div>
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer ${v.key} ?`} description="Les ressources qui la référencent recevront une valeur vide au prochain déploiement." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </div>
  )
}
