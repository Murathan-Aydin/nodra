import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { CheckIcon, PencilIcon, PlusIcon, ShieldCheckIcon, Trash2Icon, XIcon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { del, get, patch, post, teamPath } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { meQuery, permissionCatalogQuery } from "@/lib/queries"
import type { ApprovalRequest, ApprovalRule, Scope } from "@/lib/types"
import { approvalsQuery, membersQuery, rulesQuery } from "@/features/team/queries"
import { ScopePicker, useScopeNames } from "@/features/team/scope-picker"

type Tab = "pending" | "history" | "rules"

export const Route = createFileRoute("/t/$team/approvals/")({
  validateSearch: (s: Record<string, unknown>): { tab?: Tab; id?: string } => ({
    tab: s.tab === "history" || s.tab === "rules" ? s.tab : undefined,
    id: typeof s.id === "string" ? s.id : undefined,
  }),
  component: Approvals,
})

const ACTION_LABELS: Record<string, string> = {
  "resource.deploy": "Déployer / démarrer une ressource",
  "resource.delete": "Supprimer une ressource",
  "resource.env.write": "Modifier les variables",
  "resource.backup.restore": "Restaurer une sauvegarde",
  "environment.delete": "Supprimer un environnement",
  "project.delete": "Supprimer un projet",
  "server.delete": "Supprimer un serveur",
}

function Approvals() {
  const { team } = Route.useParams()
  const { tab = "pending", id } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <>
      <PageHeader title="Approbations" description="Actions sensibles soumises à l'accord d'autres membres avant exécution." />
      <Tabs value={tab} onValueChange={(v) => void navigate({ search: { tab: v as Tab }, replace: true })}>
        <TabsList className="mb-4">
          <TabsTrigger value="pending">En attente</TabsTrigger>
          <TabsTrigger value="history">Historique</TabsTrigger>
          <TabsTrigger value="rules">Règles</TabsTrigger>
        </TabsList>
        <TabsContent value="pending">
          <Requests team={team} status="pending" onOpen={(r) => void navigate({ search: { tab, id: r } })} />
        </TabsContent>
        <TabsContent value="history">
          <Requests team={team} status="all" onOpen={(r) => void navigate({ search: { tab, id: r } })} />
        </TabsContent>
        <TabsContent value="rules">
          <Rules team={team} />
        </TabsContent>
      </Tabs>
      <RequestDialog team={team} id={id} onClose={() => void navigate({ search: { tab }, replace: true })} />
    </>
  )
}

function Requests({ team, status, onOpen }: { team: string; status: string; onOpen: (id: string) => void }) {
  const requests = useQuery(approvalsQuery(team, status))
  const members = useQuery(membersQuery(team))
  const who = (id: string | null) => members.data?.find((m) => m.user_id === id)?.email ?? "—"
  const list = status === "all" ? requests.data?.filter((r) => r.status !== "pending") : requests.data
  if (!list?.length) return <EmptyState icon={ShieldCheckIcon} title={status === "pending" ? "Aucune demande en attente" : "Aucune demande traitée"} />
  return (
    <Card>
      <ul className="divide-y">
        {list.map((r) => {
          const approvals = r.votes?.filter((v) => v.decision === "approve").length ?? 0
          return (
            <li key={r.id}>
              <button type="button" onClick={() => onOpen(r.id)} className="flex w-full flex-wrap items-center gap-3 px-4 py-3 text-left text-sm hover:bg-accent/40">
                <span className="min-w-0 flex-1">
                  <span className="block font-medium">{r.summary}</span>
                  <span className="block text-xs text-muted-foreground">
                    {ACTION_LABELS[r.action] ?? r.action} · par {who(r.requested_by)} · {relativeTime(r.created_at)}
                  </span>
                </span>
                {r.status === "pending" && (
                  <Badge variant="outline">
                    {approvals}/{r.required_approvals} approbation(s)
                  </Badge>
                )}
                <StatusBadge status={r.status} />
              </button>
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

function RequestDialog({ team, id, onClose }: { team: string; id?: string; onClose: () => void }) {
  const me = useQuery(meQuery).data
  const names = useScopeNames(team)
  const request = useQuery({ queryKey: ["team", team, "approvals", "detail", id], queryFn: () => get<ApprovalRequest>(teamPath(team, `/approvals/${id}`)), enabled: !!id })
  const r = request.data
  const canApprove = usePermission(team, "approvals.approve", r?.scope.type === "team" ? undefined : r?.scope)
  const canManage = usePermission(team, "approvals.manage")
  const [comment, setComment] = useState("")
  const decide = useApiMutation((d: "approve" | "reject" | "cancel") => post<ApprovalRequest>(teamPath(team, `/approvals/${id}/${d}`), d === "cancel" ? {} : { comment }), {
    success: (res, d) => (d === "cancel" ? "Demande annulée" : d === "reject" ? "Demande rejetée" : res?.status === "executed" ? "Approuvée et exécutée" : "Approbation enregistrée"),
    invalidate: [["team", team, "approvals"]],
    onSuccess: () => setComment(""),
  })
  const mine = r?.requested_by === me?.user.id
  const voted = r?.votes?.some((v) => v.user_id === me?.user.id)
  return (
    <Dialog open={!!id} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{r?.summary ?? "Demande d'approbation"}</DialogTitle>
          <DialogDescription>{r && `${ACTION_LABELS[r.action] ?? r.action} · ${names.label(r.scope)}`}</DialogDescription>
        </DialogHeader>
        {r && (
          <div className="space-y-4 text-sm">
            <div className="flex flex-wrap items-center gap-3">
              <StatusBadge status={r.status} />
              <span className="text-muted-foreground">
                {r.votes?.filter((v) => v.decision === "approve").length ?? 0}/{r.required_approvals} approbation(s) · expire le {dateTime(r.expires_at)}
              </span>
            </div>
            {r.error && <p className="text-destructive">{r.error}</p>}
            {!!r.votes?.length && (
              <ul className="divide-y rounded-lg border">
                {r.votes.map((v) => (
                  <li key={v.user_id} className="flex items-start gap-2 px-3 py-2">
                    {v.decision === "approve" ? <CheckIcon className="mt-0.5 size-4 text-success" /> : <XIcon className="mt-0.5 size-4 text-destructive" />}
                    <span className="min-w-0 flex-1">
                      <span className="block">
                        {v.email} <span className="text-xs text-muted-foreground">{relativeTime(v.created_at)}</span>
                      </span>
                      {v.comment && <span className="block text-muted-foreground">« {v.comment} »</span>}
                    </span>
                  </li>
                ))}
              </ul>
            )}
            {r.status === "pending" && canApprove && !mine && !voted && (
              <Field label="Commentaire (facultatif)">
                <Textarea rows={2} value={comment} onChange={(e) => setComment(e.target.value)} />
              </Field>
            )}
            {r.status === "pending" && mine && <p className="text-muted-foreground">Vous ne pouvez pas approuver votre propre demande.</p>}
          </div>
        )}
        {r?.status === "pending" && (
          <DialogFooter>
            {(mine || canManage) && (
              <Button variant="ghost" loading={decide.isPending && decide.variables === "cancel"} onClick={() => decide.mutate("cancel")}>
                Annuler la demande
              </Button>
            )}
            {canApprove && !mine && !voted && (
              <>
                <Button variant="outline" loading={decide.isPending && decide.variables === "reject"} onClick={() => decide.mutate("reject")}>
                  <XIcon /> Rejeter
                </Button>
                <Button loading={decide.isPending && decide.variables === "approve"} onClick={() => decide.mutate("approve")}>
                  <CheckIcon /> Approuver
                </Button>
              </>
            )}
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}

type RuleDraft = { id?: string; name: string; scope: Scope; actions: string[]; required_approvals: number; enabled: boolean }

function Rules({ team }: { team: string }) {
  const rules = useQuery(rulesQuery(team))
  const catalog = useQuery(permissionCatalogQuery)
  const names = useScopeNames(team)
  const canManage = usePermission(team, "approvals.manage")
  const [draft, setDraft] = useState<RuleDraft | null>(null)
  const [deleting, setDeleting] = useState<ApprovalRule | null>(null)
  const key = ["team", team, "approval-rules"]
  const save = useApiMutation(
    (d: RuleDraft) => {
      const body = { name: d.name.trim(), actions: d.actions, required_approvals: d.required_approvals, enabled: d.enabled }
      return d.id ? patch(teamPath(team, `/approval-rules/${d.id}`), body) : post(teamPath(team, "/approval-rules"), { ...body, scope: d.scope })
    },
    { success: "Règle enregistrée", invalidate: [key], onSuccess: () => setDraft(null) },
  )
  // Le PATCH remplace toute la règle (nom, actions, nombre d'approbations, activation).
  const toggle = useApiMutation(
    (r: ApprovalRule) => patch(teamPath(team, `/approval-rules/${r.id}`), { name: r.name, actions: r.actions, required_approvals: r.required_approvals, enabled: !r.enabled }),
    { invalidate: [key] },
  )
  const remove = useApiMutation((rid: string) => del(teamPath(team, `/approval-rules/${rid}`)), { success: "Règle supprimée", invalidate: [key], onSuccess: () => setDeleting(null) })
  const actions = catalog.data?.gateable_actions ?? Object.keys(ACTION_LABELS)
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">Ex. : tout déploiement en production demande 1 approbation. Un seul rejet refuse la demande ; expiration après 7 jours.</p>
        {canManage && (
          <Button size="sm" onClick={() => setDraft({ name: "", scope: { type: "environment", id: "" }, actions: ["resource.deploy"], required_approvals: 1, enabled: true })}>
            <PlusIcon /> Nouvelle règle
          </Button>
        )}
      </div>
      {!rules.data?.length ? (
        <EmptyState icon={ShieldCheckIcon} title="Aucune règle" description="Sans règle, les actions s'exécutent directement (selon les droits de chacun)." />
      ) : (
        <Card>
          <ul className="divide-y">
            {rules.data.map((r) => (
              <li key={r.id} className="flex flex-wrap items-center gap-3 px-4 py-3 text-sm">
                <span className="min-w-0 flex-1">
                  <span className="block font-medium">{r.name}</span>
                  <span className="block text-xs text-muted-foreground">
                    {names.label(r.scope)} · {r.required_approvals} approbation(s) · {r.actions.map((a) => ACTION_LABELS[a] ?? a).join(", ")}
                  </span>
                </span>
                <Switch checked={r.enabled} onCheckedChange={() => toggle.mutate(r)} disabled={!canManage} aria-label={`Activer ${r.name}`} />
                {canManage && (
                  <>
                    <Button variant="ghost" size="icon-sm" aria-label={`Modifier ${r.name}`} onClick={() => setDraft({ ...r })}>
                      <PencilIcon />
                    </Button>
                    <Button variant="ghost" size="icon-sm" aria-label={`Supprimer ${r.name}`} onClick={() => setDeleting(r)}>
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </li>
            ))}
          </ul>
        </Card>
      )}
      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{draft?.id ? "Modifier la règle" : "Nouvelle règle"}</DialogTitle>
          </DialogHeader>
          {draft && (
            <div className="space-y-4">
              <Field label="Nom">
                <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="ex. Production protégée" autoFocus />
              </Field>
              <div className="space-y-1.5">
                <p className="text-sm font-medium">Portée</p>
                {draft.id ? (
                  <p className="text-sm text-muted-foreground">{names.label(draft.scope)}</p>
                ) : (
                  <ScopePicker team={team} value={draft.scope} onChange={(s) => setDraft({ ...draft, scope: s })} />
                )}
              </div>
              <fieldset className="space-y-2">
                <legend className="mb-2 text-sm font-medium">Actions soumises</legend>
                {actions.map((a) => (
                  <Label key={a} className="font-normal">
                    <Checkbox
                      checked={draft.actions.includes(a)}
                      onCheckedChange={(c) => setDraft({ ...draft, actions: c === true ? [...draft.actions, a] : draft.actions.filter((x) => x !== a) })}
                    />
                    {ACTION_LABELS[a] ?? a}
                  </Label>
                ))}
              </fieldset>
              <div className="flex flex-wrap items-end gap-6">
                <Field label="Approbations requises">
                  <Input
                    type="number"
                    min={1}
                    max={10}
                    className="w-24"
                    value={draft.required_approvals}
                    onChange={(e) => setDraft({ ...draft, required_approvals: Number(e.target.value) })}
                  />
                </Field>
                <Label className="pb-2 font-normal">
                  <Switch checked={draft.enabled} onCheckedChange={(v) => setDraft({ ...draft, enabled: v })} /> Active
                </Label>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button
              loading={save.isPending}
              disabled={!draft?.name.trim() || !draft.actions.length || (draft.scope.type !== "team" && !draft.scope.id)}
              onClick={() => draft && save.mutate(draft)}
            >
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Supprimer la règle ${deleting?.name} ?`}
        description="Les demandes en attente restent à traiter."
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </div>
  )
}
