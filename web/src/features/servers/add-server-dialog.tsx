import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { KeyRoundIcon } from "lucide-react"
import { CopyButton } from "@/components/copy-button"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import type { Server, ServerTask } from "@/lib/types"
import { KeyForm } from "./key-form"
import { keysQuery } from "./queries"

/** Ajout d'un serveur : clé SSH (existante, générée ou importée), connexion, proxy ; la validation démarre aussitôt. */
export function AddServerDialog({ team, open, onOpenChange }: { team: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const navigate = useNavigate()
  const keys = useQuery({ ...keysQuery(team), enabled: open })
  const [form, setForm] = useState({ name: "", ip: "", port: "22", user: "root", private_key_id: "", proxy_type: "traefik", description: "" })
  const [newKey, setNewKey] = useState(false)
  const key = keys.data?.find((k) => k.id === form.private_key_id)
  const create = useApiMutation(
    (v: typeof form) =>
      post<{ server: Server; task: ServerTask }>(teamPath(team, "/servers"), { ...v, port: Number(v.port) || 22 }),
    {
      success: "Serveur ajouté : validation en cours",
      invalidate: [["team", team, "servers"]],
      onSuccess: (res) => {
        onOpenChange(false)
        void navigate({ to: "/t/$team/servers/$server/tasks", params: { team, server: res.server.id }, search: { task: res.task.id } })
      },
    },
  )
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Ajouter un serveur</DialogTitle>
          <DialogDescription>Nodra s'y connecte en SSH avec une clé ; Docker peut ensuite être installé automatiquement.</DialogDescription>
        </DialogHeader>
        {newKey ? (
          <div className="space-y-3 rounded-lg border p-4">
            <p className="text-sm font-medium">Nouvelle clé SSH</p>
            <KeyForm
              team={team}
              onCreated={(k) => {
                setForm((f) => ({ ...f, private_key_id: k.id }))
                setNewKey(false)
              }}
            />
            <Button variant="ghost" size="sm" onClick={() => setNewKey(false)}>
              Annuler
            </Button>
          </div>
        ) : (
          <form
            id="add-server"
            className="grid gap-4 sm:grid-cols-6"
            onSubmit={(e) => {
              e.preventDefault()
              create.mutate(form)
            }}
          >
            <Field label="Nom" htmlFor="sv-name" className="sm:col-span-6">
              <Input id="sv-name" required value={form.name} onChange={set("name")} placeholder="ex. production-1" />
            </Field>
            <Field label="Adresse IP ou nom d'hôte" htmlFor="sv-ip" className="sm:col-span-3">
              <Input id="sv-ip" required value={form.ip} onChange={set("ip")} placeholder="203.0.113.10" />
            </Field>
            <Field label="Port SSH" htmlFor="sv-port" className="sm:col-span-1">
              <Input id="sv-port" inputMode="numeric" value={form.port} onChange={set("port")} />
            </Field>
            <Field label="Utilisateur" htmlFor="sv-user" className="sm:col-span-2">
              <Input id="sv-user" value={form.user} onChange={set("user")} />
            </Field>
            <Field label="Clé SSH" className="sm:col-span-4">
              <Select value={form.private_key_id} onValueChange={(v) => setForm({ ...form, private_key_id: v })}>
                <SelectTrigger>
                  <SelectValue placeholder={keys.data?.length ? "Choisir une clé" : "Aucune clé : créez-en une"} />
                </SelectTrigger>
                <SelectContent>
                  {keys.data?.map((k) => (
                    <SelectItem key={k.id} value={k.id}>
                      {k.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <div className="flex items-end sm:col-span-2">
              <Button type="button" variant="outline" className="w-full" onClick={() => setNewKey(true)}>
                <KeyRoundIcon /> Nouvelle clé
              </Button>
            </div>
            {key && (
              <div className="space-y-1.5 sm:col-span-6">
                <p className="text-xs text-muted-foreground">
                  Ajoutez cette clé publique à <code className="font-mono">~/.ssh/authorized_keys</code> de l'utilisateur sur le serveur :
                </p>
                <div className="flex items-start gap-1 rounded-md border bg-muted/40 p-2">
                  <code className="min-w-0 flex-1 font-mono text-xs break-all">{key.public_key}</code>
                  <CopyButton value={key.public_key} />
                </div>
              </div>
            )}
            <Field label="Proxy" className="sm:col-span-3" hint="Sert les domaines des applications (HTTPS automatique).">
              <Select value={form.proxy_type} onValueChange={(v) => setForm({ ...form, proxy_type: v })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="traefik">Traefik</SelectItem>
                  <SelectItem value="caddy">Caddy</SelectItem>
                  <SelectItem value="none">Aucun</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Description" htmlFor="sv-desc" className="sm:col-span-3">
              <Input id="sv-desc" value={form.description} onChange={set("description")} />
            </Field>
          </form>
        )}
        {!newKey && (
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Annuler
            </Button>
            <Button type="submit" form="add-server" loading={create.isPending} disabled={!form.private_key_id}>
              Ajouter et valider
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}
