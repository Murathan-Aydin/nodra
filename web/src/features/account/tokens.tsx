import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { KeyRoundIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { CopyButton } from "@/components/copy-button"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { del, get, post } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { teamsQuery } from "@/lib/queries"
import type { APIToken } from "@/lib/types"

const SCOPES: [string, string][] = [
  ["read", "Lecture"],
  ["write", "Écriture (créer, modifier, supprimer)"],
  ["deploy", "Déploiements et démarrages"],
  ["root", "Tous les droits de votre compte"],
]

/** Jetons API personnels : création (affiché une seule fois), révocation, exemples d'usage (F2-04). */
export function TokensSection({ team }: { team: string }) {
  const tokens = useQuery({ queryKey: ["me", "tokens"], queryFn: () => get<APIToken[]>("/me/tokens") })
  const teams = useQuery(teamsQuery)
  const teamName = (id: string | null) => teams.data?.find((t) => t.id === id)?.name ?? "—"
  const [draft, setDraft] = useState<{ name: string; team_id: string; scopes: string[]; days: string } | null>(null)
  const [created, setCreated] = useState<string | null>(null)
  const [revoking, setRevoking] = useState<APIToken | null>(null)
  const create = useApiMutation(
    (d: NonNullable<typeof draft>) =>
      post<{ token: string; api_token: APIToken }>("/me/tokens", { name: d.name.trim(), team_id: d.team_id, scopes: d.scopes, expires_in_days: d.days ? Number(d.days) : undefined }),
    {
      invalidate: [["me", "tokens"]],
      onSuccess: (r) => {
        setDraft(null)
        setCreated(r.token)
      },
    },
  )
  const revoke = useApiMutation((id: string) => del(`/me/tokens/${id}`), { success: "Jeton révoqué", invalidate: [["me", "tokens"]], onSuccess: () => setRevoking(null) })
  const origin = window.location.origin
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-4">
        <div className="space-y-1.5">
          <CardTitle>Jetons API</CardTitle>
          <CardDescription>
            Pour l'API REST (<code className="font-mono text-xs">Authorization: Bearer …</code>), la CI et le serveur MCP. Un jeton agit sur une équipe avec au plus vos droits.
          </CardDescription>
        </div>
        <Button size="sm" onClick={() => setDraft({ name: "", team_id: team, scopes: ["read"], days: "90" })}>
          <PlusIcon /> Nouveau jeton
        </Button>
      </CardHeader>
      <CardContent>
        {!tokens.data?.length ? (
          <EmptyState icon={KeyRoundIcon} title="Aucun jeton" />
        ) : (
          <ul className="divide-y rounded-lg border">
            {tokens.data.map((t) => {
              const expired = !!t.expires_at && new Date(t.expires_at) < new Date()
              return (
                <li key={t.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5 text-sm">
                  <KeyRoundIcon className="size-4 text-muted-foreground" />
                  <span className="min-w-0 flex-1">
                    <span className="flex flex-wrap items-center gap-1.5">
                      <span className="font-medium">{t.name}</span>
                      {t.scopes.map((s) => (
                        <Badge key={s} variant={s === "root" ? "warning" : "outline"}>
                          {s}
                        </Badge>
                      ))}
                      {expired && <Badge variant="destructive">expiré</Badge>}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {teamName(t.team_id)} · utilisé {t.last_used_at ? relativeTime(t.last_used_at) : "jamais"} · {t.expires_at ? `expire le ${dateTime(t.expires_at)}` : "sans expiration"}
                    </span>
                  </span>
                  <Button variant="ghost" size="icon-sm" aria-label={`Révoquer ${t.name}`} onClick={() => setRevoking(t)}>
                    <Trash2Icon />
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>

      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Nouveau jeton</DialogTitle>
          </DialogHeader>
          {draft && (
            <div className="space-y-4">
              <Field label="Nom">
                <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="ex. GitHub Actions" autoFocus />
              </Field>
              <Field label="Équipe">
                <Select value={draft.team_id} onValueChange={(v) => setDraft({ ...draft, team_id: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {teams.data?.map((t) => (
                      <SelectItem key={t.id} value={t.id}>
                        {t.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <fieldset className="space-y-2">
                <legend className="mb-2 text-sm font-medium">Portée</legend>
                {SCOPES.map(([s, label]) => (
                  <Label key={s} className="font-normal">
                    <Checkbox
                      checked={draft.scopes.includes(s)}
                      onCheckedChange={(c) => setDraft({ ...draft, scopes: c === true ? [...draft.scopes, s] : draft.scopes.filter((x) => x !== s) })}
                    />
                    <code className="font-mono text-xs">{s}</code> {label}
                  </Label>
                ))}
              </fieldset>
              <Field label="Expiration (jours)" hint="Vide : jamais.">
                <Input type="number" min={1} className="w-32" value={draft.days} onChange={(e) => setDraft({ ...draft, days: e.target.value })} />
              </Field>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button loading={create.isPending} disabled={!draft?.name.trim() || !draft.scopes.length} onClick={() => draft && create.mutate(draft)}>
              Créer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!created} onOpenChange={(o) => !o && setCreated(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Jeton créé</DialogTitle>
            <DialogDescription>Copiez-le maintenant : il ne sera plus jamais affiché.</DialogDescription>
          </DialogHeader>
          {created && (
            <div className="space-y-4">
              <Snippet code={created} />
              <div className="space-y-1.5">
                <p className="text-sm font-medium">Exemple</p>
                <Snippet code={`curl -H "Authorization: Bearer ${created}" ${origin}/api/v1/teams/${team}/resources`} />
              </div>
              <div className="space-y-1.5">
                <p className="text-sm font-medium">Client MCP (si activé sur l'instance)</p>
                <Snippet code={JSON.stringify({ mcpServers: { nodra: { type: "http", url: `${origin}/api/v1/mcp`, headers: { Authorization: `Bearer ${created}` } } } }, null, 2)} />
              </div>
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setCreated(null)}>J'ai copié le jeton</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!revoking}
        onOpenChange={(o) => !o && setRevoking(null)}
        title={`Révoquer « ${revoking?.name} » ?`}
        description="Les scripts et intégrations qui l'utilisent seront refusés immédiatement."
        confirmLabel="Révoquer"
        destructive
        loading={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking.id)}
      />
    </Card>
  )
}

function Snippet({ code }: { code: string }) {
  return (
    <div className="flex items-start gap-2 rounded-lg bg-muted p-3">
      <pre className="flex-1 font-mono text-xs break-all whitespace-pre-wrap">{code}</pre>
      <CopyButton value={code} />
    </div>
  )
}
