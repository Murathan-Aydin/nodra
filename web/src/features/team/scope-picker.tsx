import { useQuery } from "@tanstack/react-query"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import type { Scope } from "@/lib/types"
import { projectsQuery, resourcesQuery } from "@/features/projects/queries"
import { useEnvironments } from "@/features/resources/locations"
import { serversQuery } from "@/features/servers/queries"

export const SCOPE_TYPES: Record<string, string> = {
  team: "Équipe entière",
  project: "Projet",
  environment: "Environnement",
  server: "Serveur",
  resource: "Ressource",
}

/** Noms lisibles des portées de l'équipe (projets, environnements, serveurs, ressources). */
export function useScopeNames(team: string) {
  const projects = useQuery(projectsQuery(team)).data ?? []
  const envs = useEnvironments(team)
  const servers = useQuery(serversQuery(team)).data ?? []
  const resources = useQuery(resourcesQuery(team)).data ?? []
  const names = new Map<string, string>()
  for (const p of projects) names.set(p.id, p.name)
  for (const e of envs) names.set(e.id, `${e.project.name} / ${e.name}`)
  for (const s of servers) names.set(s.id, s.name)
  for (const r of resources) names.set(r.id, `${r.name} (${r.project_name} / ${r.environment_name})`)
  const options = (type: string) =>
    type === "project"
      ? projects.map((p) => ({ id: p.id, name: p.name }))
      : type === "environment"
        ? envs.map((e) => ({ id: e.id, name: `${e.project.name} / ${e.name}` }))
        : type === "server"
          ? servers.map((s) => ({ id: s.id, name: s.name }))
          : type === "resource"
            ? resources.map((r) => ({ id: r.id, name: `${r.name} (${r.project_name} / ${r.environment_name})` }))
            : []
  return {
    label: (s: Scope | null | undefined) => (!s || s.type === "team" ? SCOPE_TYPES.team : `${SCOPE_TYPES[s.type] ?? s.type} : ${names.get(s.id) ?? s.id.slice(0, 8)}`),
    options,
  }
}

/** Choix d'une portée : type puis élément (équipe, projet, environnement, serveur, ressource). */
export function ScopePicker({ team, value, onChange }: { team: string; value: Scope; onChange: (s: Scope) => void }) {
  const { options } = useScopeNames(team)
  const items = options(value.type)
  return (
    <div className="grid gap-2 sm:grid-cols-[12rem_1fr]">
      <Select value={value.type} onValueChange={(type) => onChange({ type, id: type === "team" ? team : "" })}>
        <SelectTrigger aria-label="Type de portée">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {Object.entries(SCOPE_TYPES).map(([k, v]) => (
            <SelectItem key={k} value={k}>
              {v}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {value.type !== "team" && (
        <Select value={value.id} onValueChange={(id) => onChange({ ...value, id })}>
          <SelectTrigger aria-label="Élément">
            <SelectValue placeholder={items.length ? "Choisir…" : "Aucun élément"} />
          </SelectTrigger>
          <SelectContent>
            {items.map((i) => (
              <SelectItem key={i.id} value={i.id}>
                {i.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  )
}
