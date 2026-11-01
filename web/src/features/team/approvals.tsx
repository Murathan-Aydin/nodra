import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { CheckIcon, PlusIcon, Trash2Icon, XIcon } from "lucide-react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { api, get, post } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { catalogQuery, usePermission } from "@/lib/permissions"
import { teamQueries } from "@/lib/queries"

type Rule = { id: string; name: string; scope: { type: string; id: string } | null; actions: string[]; required_approvals: number; enabled: boolean }
type Request = { id: string; action: string; status: string; summary: string; requested_by: string | null; error: string; required_approvals: number; votes: { decision?: string; email?: string }[] | null; created_at: string; expires_at: string }

const ACTION: Record<string, string> = {
  "resource.deploy": "Déployer une ressource",
  "resource.delete": "Supprimer une ressource",
  "resource.env.write": "Modifier des variables",
  "resource.backup.restore": "Restaurer une sauvegarde",
  "environment.delete": "Supprimer un environnement",
  "project.delete": "Supprimer un projet",
  "server.delete": "Supprimer un serveur",
}

const STATUS: Record<string, { label: string; dot: string }> = {
  pending: { label: "En attente", dot: "bg-warning animate-pulse" },
  approved: { label: "Approuvée", dot: "bg-success" },
  rejected: { label: "Refusée", dot: "bg-destructive" },
  cancelled: { label: "Annulée", dot: "bg-muted-foreground" },
  executed: { label: "Exécutée", dot: "bg-success" },
  failed: { label: "Échouée", dot: "bg-destructive" },
  expired: { label: "Expirée", dot: "bg-muted-foreground" },
}

/** Page « Équipe › Approbations » : demandes en attente et règles (N approbations par action et portée). */
export function ApprovalsPage({ team }: { team: string }) {
  return (
    <>
      <PageHeader title="Approbations" description="Les actions couvertes par une règle attendent d'être approuvées avant d'être exécutées." />
      <div className="space-y-6">
        <Requests team={team} />
        <Rules team={team} />
      </div>
    </>
  )
}

function Requests({ team }: { team: string }) {
  const qc = useQueryClient()
  const [status, setStatus] = useState("pending")
  const key = ["team", team, "approvals", status]
  const list = useQuery({ queryKey: key, queryFn: () => get<Request[]>(`/teams/${team}/approvals${status === "all" ? "" : `?status=${status}`}`), refetchInterval: 15_000 })
  const act = useMutation({
    mutationFn: ({ id, a }: { id: string; a: "approve" | "reject" | "cancel" }) => post(`/teams/${team}/approvals/${id}/${a}`, {}),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["team", team, "approvals"] }),
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title="Demandes"
      actions={
        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger className="h-8 w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Toutes</SelectItem>
            {Object.entries(STATUS).map(([k, v]) => (
              <SelectItem key={k} value={k}>
                {v.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      }
      bodyClassName="p-0"
    >
      {list.data?.map((r) => {
        const s = STATUS[r.status] ?? STATUS.pending
        return (
          <div key={r.id} className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {ACTION[r.action] ?? r.action}
                {r.summary && ` · ${r.summary}`}
              </p>
              <p className="truncate text-xs text-muted-foreground" title={dateTime(r.created_at)}>
                {r.requested_by ? "Demandée" : "Webhook"} {relativeTime(r.created_at)} · {(r.votes ?? []).filter((v) => v.decision === "approve").length}/{r.required_approvals} approbation(s) · expire {relativeTime(r.expires_at)}
                {r.error && ` · ${r.error}`}
              </p>
            </div>
            <Pill label={s.label} dot={s.dot} />
            {r.status === "pending" && (
              <>
                <Button size="sm" variant="outline" onClick={() => act.mutate({ id: r.id, a: "approve" })}>
                  <CheckIcon /> Approuver
                </Button>
                <Button size="sm" variant="ghost" onClick={() => act.mutate({ id: r.id, a: "reject" })}>
                  <XIcon /> Refuser
                </Button>
                <Button size="sm" variant="ghost" onClick={() => act.mutate({ id: r.id, a: "cancel" })}>
                  Annuler
                </Button>
              </>
            )}
          </div>
        )
      })}
      {list.data?.length === 0 && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune demande.</p>}
    </SettingsCard>
  )
}

function Rules({ team }: { team: string }) {
  const qc = useQueryClient()
  const manage = usePermission(team, "approvals.manage")
  const gateable = useQuery(catalogQuery).data?.gateable_actions ?? []
  const projects = useQuery(teamQueries.projects(team)).data ?? []
  const rules = useQuery({ queryKey: ["team", team, "approval-rules"], queryFn: () => get<Rule[]>(`/teams/${team}/approval-rules`) })
  const [adding, setAdding] = useState(false)
  const [d, setD] = useState({ name: "", scope: "team", actions: ["resource.deploy"], required_approvals: "1" })
  const refresh = () => void qc.invalidateQueries({ queryKey: ["team", team, "approval-rules"] })
  const add = useMutation({
    mutationFn: () => post(`/teams/${team}/approval-rules`, { name: d.name, actions: d.actions, required_approvals: Number(d.required_approvals), ...(d.scope !== "team" ? { scope: { type: "project", id: d.scope } } : {}) }),
    onSuccess: () => (setAdding(false), refresh()),
    onError: (e) => toast.error(e.message),
  })
  const toggle = useMutation({ mutationFn: (r: Rule) => api(`/teams/${team}/approval-rules/${r.id}`, { method: "PATCH", json: { enabled: !r.enabled } }), onSuccess: refresh, onError: (e) => toast.error(e.message) })
  const remove = useMutation({ mutationFn: (id: string) => api(`/teams/${team}/approval-rules/${id}`, { method: "DELETE" }), onSuccess: refresh, onError: (e) => toast.error(e.message) })
  return (
    <SettingsCard
      title="Règles"
      actions={
        manage && (
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            <PlusIcon /> Nouvelle règle
          </Button>
        )
      }
      bodyClassName="p-0"
    >
      {adding && (
        <div className="space-y-4 border-b border-border p-5">
          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="Nom">
              <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Production" />
            </Field>
            <Field label="Portée">
              <Select value={d.scope} onValueChange={(v) => setD({ ...d, scope: v })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="team">Toute l'équipe</SelectItem>
                  {projects.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      Projet {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Approbations requises">
              <Input value={d.required_approvals} onChange={(e) => setD({ ...d, required_approvals: e.target.value.replace(/\D/g, "") })} />
            </Field>
          </div>
          <div className="grid gap-1.5 sm:grid-cols-2">
            {gateable.map((a) => (
              <label key={a} className="flex items-center gap-2 text-sm">
                <Checkbox checked={d.actions.includes(a)} onCheckedChange={(c) => setD({ ...d, actions: c ? [...d.actions, a] : d.actions.filter((x) => x !== a) })} />
                {ACTION[a] ?? a}
              </label>
            ))}
          </div>
          <div className="flex gap-2">
            <Button size="sm" disabled={!d.name || d.actions.length === 0} loading={add.isPending} onClick={() => add.mutate()}>
              Créer
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setAdding(false)}>
              Annuler
            </Button>
          </div>
        </div>
      )}
      {rules.data?.map((r) => (
        <div key={r.id} className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{r.name}</p>
            <p className="truncate text-xs text-muted-foreground">
              {r.actions.map((a) => ACTION[a] ?? a).join(", ")} · {r.required_approvals} approbation(s)
              {r.scope ? ` · ${projects.find((p) => p.id === r.scope?.id)?.name ?? r.scope.type}` : " · toute l'équipe"}
            </p>
          </div>
          {manage && <Switch checked={r.enabled} onCheckedChange={() => toggle.mutate(r)} />}
          {manage && (
            <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => remove.mutate(r.id)}>
              <Trash2Icon />
            </Button>
          )}
        </div>
      ))}
      {!adding && rules.data?.length === 0 && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucune règle : toutes les actions s'exécutent directement.</p>}
    </SettingsCard>
  )
}
