import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { KeyRoundIcon, PlusIcon, SparklesIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { api, get, post } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import type { PrivateKey } from "@/lib/types"

/** Page « Clés SSH » (Private Keys du fork) : génération, import, renommage, suppression. */
export function PrivateKeysPage({ team }: { team: string }) {
  const qc = useQueryClient()
  const key = ["team", team, "private-keys"]
  const keys = useQuery({ queryKey: key, queryFn: () => get<PrivateKey[]>(`/teams/${team}/private-keys`) })
  const [adding, setAdding] = useState(false)
  const [d, setD] = useState({ name: "", description: "", private_key: "" })
  const [deleting, setDeleting] = useState<PrivateKey | null>(null)
  const create = useMutation({
    mutationFn: () => post<PrivateKey>(`/teams/${team}/private-keys`, d),
    onSuccess: () => {
      toast.success(d.private_key ? "Clé importée" : "Clé générée")
      setAdding(false)
      setD({ name: "", description: "", private_key: "" })
      void qc.invalidateQueries({ queryKey: key })
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: (k: PrivateKey) => api(`/teams/${team}/private-keys/${k.id}`, { method: "DELETE" }),
    onSuccess: () => {
      setDeleting(null)
      void qc.invalidateQueries({ queryKey: key })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <>
      <PageHeader
        title="Clés SSH"
        description="Clés utilisées pour se connecter aux serveurs et cloner des dépôts privés (deploy keys)."
        actions={
          <Button size="sm" onClick={() => setAdding(true)}>
            <PlusIcon /> Ajouter
          </Button>
        }
      />
      <div className="space-y-4">
        {keys.data?.map((k) => <KeyCard key={k.id} team={team} k={k} onDelete={() => setDeleting(k)} />)}
        {keys.data?.length === 0 && <p className="text-sm text-muted-foreground">Aucune clé.</p>}
      </div>
      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Nouvelle clé SSH</DialogTitle>
            <DialogDescription>Laissez la clé privée vide pour générer une paire ed25519.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <Field label="Nom *">
              <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
            </Field>
            <Field label="Description">
              <Input value={d.description} onChange={(e) => setD({ ...d, description: e.target.value })} />
            </Field>
            <Field label="Clé privée (PEM ou OpenSSH, sans phrase de passe)">
              <Textarea value={d.private_key} onChange={(e) => setD({ ...d, private_key: e.target.value })} rows={6} className="font-mono" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" />
            </Field>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setAdding(false)}>
              Annuler
            </Button>
            <Button disabled={!d.name.trim()} loading={create.isPending} onClick={() => create.mutate()}>
              {d.private_key ? "Importer" : (
                <>
                  <SparklesIcon /> Générer
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={`Supprimer « ${deleting?.name} » ?`} description="Refusé si un serveur utilise cette clé." actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => deleting && remove.mutate(deleting)} />
    </>
  )
}

function KeyCard({ team, k, onDelete }: { team: string; k: PrivateKey; onDelete: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState(k.name)
  const [description, setDescription] = useState(k.description ?? "")
  const save = useMutation({
    mutationFn: () => api(`/teams/${team}/private-keys/${k.id}`, { method: "PATCH", json: { name, description } }),
    onSuccess: () => (toast.success("Clé enregistrée"), void qc.invalidateQueries({ queryKey: ["team", team, "private-keys"] })),
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title={k.name}
      description={`${k.fingerprint}${k.created_at ? ` · créée le ${dateTime(k.created_at)}` : ""}`}
      actions={
        <>
          <Button size="sm" variant="outline" disabled={name === k.name && description === (k.description ?? "")} loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
          <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={onDelete}>
            <Trash2Icon />
          </Button>
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Nom">
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Description">
          <Input value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <Field label="Clé publique (à ajouter dans ~/.ssh/authorized_keys)" className="sm:col-span-2">
          <CopyInput value={k.public_key} />
        </Field>
      </div>
    </SettingsCard>
  )
}

type Token = { id: string; team_id: string; name: string; scopes: string[] | null; expires_at: string | null; last_used_at: string | null; created_at: string }

const SCOPES: [string, string][] = [
  ["read", "Lecture (hors secrets)"],
  ["write", "Écriture (sans déploiement ni administration)"],
  ["deploy", "Déploiement"],
  ["root", "Tout"],
]

/** Page « Jetons API » (API Tokens du fork) : jetons rattachés à l'équipe, scopes et expiration. */
export function ApiTokensPage({ team }: { team: string }) {
  const qc = useQueryClient()
  const tokens = useQuery({ queryKey: ["me", "tokens"], queryFn: () => get<Token[]>("/me/tokens") })
  const [adding, setAdding] = useState(false)
  const [created, setCreated] = useState("")
  const [d, setD] = useState({ name: "", scopes: ["read"], expires: "90" })
  const [deleting, setDeleting] = useState<Token | null>(null)
  const create = useMutation({
    mutationFn: () => post<{ token: string }>("/me/tokens", { name: d.name, team_id: team, scopes: d.scopes, expires_in_days: d.expires === "0" ? undefined : Number(d.expires) }),
    onSuccess: (r) => {
      setCreated(r.token)
      setAdding(false)
      void qc.invalidateQueries({ queryKey: ["me", "tokens"] })
    },
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: (t: Token) => api(`/me/tokens/${t.id}`, { method: "DELETE" }),
    onSuccess: () => {
      setDeleting(null)
      void qc.invalidateQueries({ queryKey: ["me", "tokens"] })
    },
    onError: (e) => toast.error(e.message),
  })
  const list = (tokens.data ?? []).filter((t) => t.team_id === team)
  return (
    <>
      <PageHeader
        title="Jetons API"
        description="Accès à l'API (Authorization: Bearer). Les droits effectifs sont l'intersection des scopes et de vos propres droits."
        actions={
          <Button size="sm" onClick={() => setAdding(true)}>
            <PlusIcon /> Nouveau jeton
          </Button>
        }
      />
      {created && (
        <SettingsCard title="Jeton créé" description="Copiez-le maintenant : il ne sera plus affiché." className="mb-6">
          <CopyInput value={created} />
        </SettingsCard>
      )}
      <SettingsCard title="Jetons de l'équipe" bodyClassName="p-0">
        {list.map((t) => (
          <div key={t.id} className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
            <KeyRoundIcon className="size-4 text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">{t.name}</p>
              <p className="truncate text-xs text-muted-foreground">
                {(t.scopes ?? []).join(", ")} · créé {relativeTime(t.created_at)} · utilisé {relativeTime(t.last_used_at)}
              </p>
            </div>
            {t.expires_at && <Pill label={new Date(t.expires_at) < new Date() ? "Expiré" : `Expire le ${dateTime(t.expires_at)}`} dot={new Date(t.expires_at) < new Date() ? "bg-destructive" : "bg-muted-foreground"} />}
            <Button size="sm" variant="ghost" aria-label="Révoquer" onClick={() => setDeleting(t)}>
              <Trash2Icon />
            </Button>
          </div>
        ))}
        {list.length === 0 && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucun jeton pour cette équipe.</p>}
      </SettingsCard>
      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Nouveau jeton API</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <Field label="Nom *">
              <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="CI GitHub" />
            </Field>
            <div className="space-y-2">
              <p className="text-sm font-medium text-muted-foreground">Scopes</p>
              {SCOPES.map(([s, label]) => (
                <label key={s} className="flex items-center gap-2 text-sm">
                  <Checkbox checked={d.scopes.includes(s)} onCheckedChange={(c) => setD({ ...d, scopes: c ? [...d.scopes, s] : d.scopes.filter((x) => x !== s) })} />
                  <span className="font-mono text-xs">{s}</span> {label}
                </label>
              ))}
            </div>
            <Field label="Expiration">
              <Select value={d.expires} onValueChange={(v) => setD({ ...d, expires: v })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="30">30 jours</SelectItem>
                  <SelectItem value="90">90 jours</SelectItem>
                  <SelectItem value="365">1 an</SelectItem>
                  <SelectItem value="0">Jamais</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setAdding(false)}>
              Annuler
            </Button>
            <Button disabled={!d.name.trim() || d.scopes.length === 0} loading={create.isPending} onClick={() => create.mutate()}>
              Créer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={`Révoquer « ${deleting?.name} » ?`} description="Les appels qui l'utilisent seront refusés." actionLabel="Révoquer" loading={remove.isPending} onConfirm={() => deleting && remove.mutate(deleting)} />
    </>
  )
}
