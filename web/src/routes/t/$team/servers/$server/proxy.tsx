import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { FileCode2Icon, PlayIcon, PlusIcon, RotateCwIcon, ShieldIcon, SquareIcon, Trash2Icon } from "lucide-react"
import { CodeEditor } from "@/components/code-editor"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { LogStream } from "@/components/log-stream"
import { StatusBadge } from "@/components/status-badge"
import { TaskDialog } from "@/components/task-output"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { del, get, patch, post, put, teamPath } from "@/lib/api"
import { dateTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Certificate, ProxyConfig, ProxyState, ServerTask } from "@/lib/types"

export const Route = createFileRoute("/t/$team/servers/$server/proxy")({ component: ProxyPage })

function ProxyPage() {
  const { team, server } = Route.useParams()
  const base = teamPath(team, `/servers/${server}/proxy`)
  const key = ["team", team, "server", server, "proxy"]
  const proxy = useQuery({ queryKey: key, queryFn: () => get<ProxyState>(base) })
  const canManage = usePermission(team, "server.proxy.manage", { type: "server", id: server })
  const [task, setTask] = useState<ServerTask | null>(null)
  const action = useApiMutation((a: string) => post<ServerTask>(`${base}/${a}`), { invalidate: [key], onSuccess: setTask })
  const p = proxy.data
  if (!p) return null
  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="flex flex-wrap items-center gap-4 pt-5">
          <div className="flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-lg bg-accent text-accent-foreground">
              <ShieldIcon className="size-5" />
            </span>
            <div>
              <p className="font-medium capitalize">{p.type === "none" ? "Aucun proxy" : p.type}</p>
              <p className="text-xs text-muted-foreground">{p.is_custom ? "Configuration personnalisée" : "Configuration générée"}</p>
            </div>
          </div>
          {p.type !== "none" && <StatusBadge status={p.status} />}
          <div className="flex-1" />
          {canManage && p.type !== "none" && (
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => action.mutate("start")} disabled={action.isPending}>
                <PlayIcon /> Démarrer
              </Button>
              <Button variant="outline" size="sm" onClick={() => action.mutate("restart")} disabled={action.isPending}>
                <RotateCwIcon /> Redémarrer
              </Button>
              <Button variant="outline" size="sm" onClick={() => action.mutate("stop")} disabled={action.isPending}>
                <SquareIcon /> Arrêter
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
      <Tabs defaultValue="logs">
        <TabsList>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="configs">Configurations dynamiques</TabsTrigger>
          <TabsTrigger value="certs">Certificats</TabsTrigger>
          <TabsTrigger value="settings">Réglages</TabsTrigger>
        </TabsList>
        <TabsContent value="logs">
          {p.type === "none" ? (
            <p className="text-sm text-muted-foreground">Aucun proxy sur ce serveur.</p>
          ) : (
            <LogStream path={teamPath(team, `/servers/${server}/proxy/logs/stream`)} query={{ tail: 200 }} filename="proxy.log" />
          )}
        </TabsContent>
        <TabsContent value="configs">
          <Configs team={team} server={server} type={p.type} canManage={canManage} />
        </TabsContent>
        <TabsContent value="certs">
          <Certificates team={team} server={server} canManage={canManage} />
        </TabsContent>
        <TabsContent value="settings">
          <ProxySettings team={team} server={server} proxy={p} canManage={canManage} />
        </TabsContent>
      </Tabs>
      <TaskDialog team={team} task={task} onOpenChange={(o) => !o && setTask(null)} />
    </div>
  )
}

function Configs({ team, server, type, canManage }: { team: string; server: string; type: string; canManage: boolean }) {
  const base = teamPath(team, `/servers/${server}/proxy/configs`)
  const key = ["team", team, "server", server, "proxy-configs"]
  const configs = useQuery({ queryKey: key, queryFn: () => get<ProxyConfig[]>(base) })
  const [editing, setEditing] = useState<{ name: string; content: string; isNew: boolean } | null>(null)
  const [deleting, setDeleting] = useState<string | null>(null)
  const save = useApiMutation((c: { name: string; content: string }) => put(`${base}/${encodeURIComponent(c.name)}`, { content: c.content }), {
    success: "Configuration enregistrée et synchronisée",
    invalidate: [key],
    onSuccess: () => setEditing(null),
  })
  const remove = useApiMutation((name: string) => del(`${base}/${encodeURIComponent(name)}`), {
    success: "Configuration supprimée",
    invalidate: [key],
    onSuccess: () => setDeleting(null),
  })
  const ext = type === "caddy" ? ".caddy" : ".yaml"
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          Fichiers {type === "caddy" ? "Caddyfile (.caddy)" : "Traefik (.yml / .yaml)"} chargés sans redémarrage. Le préfixe <code>nodra-</code> est réservé.
        </p>
        {canManage && type !== "none" && (
          <Button size="sm" onClick={() => setEditing({ name: "", content: "", isNew: true })}>
            <PlusIcon /> Nouveau fichier
          </Button>
        )}
      </div>
      {!configs.data?.length ? (
        <p className="py-6 text-center text-sm text-muted-foreground">Aucune configuration dynamique.</p>
      ) : (
        <Card>
          <ul className="divide-y">
            {configs.data.map((c) => (
              <li key={c.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
                <FileCode2Icon className="size-4 text-muted-foreground" />
                <button className="font-mono hover:underline" onClick={() => setEditing({ name: c.name, content: c.content, isNew: false })}>
                  {c.name}
                </button>
                {c.name.startsWith("nodra-") && <Badge variant="secondary">généré</Badge>}
                <span className="ml-auto text-xs text-muted-foreground">{dateTime(c.updated_at)}</span>
                {canManage && (
                  <Button variant="ghost" size="icon-sm" onClick={() => setDeleting(c.name)} aria-label="Supprimer">
                    <Trash2Icon />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </Card>
      )}
      <Dialog open={!!editing} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{editing?.isNew ? "Nouvelle configuration" : editing?.name}</DialogTitle>
          </DialogHeader>
          {editing && (
            <div className="space-y-3">
              {editing.isNew && (
                <Field label="Nom du fichier" hint={`ex. redirections${ext}`}>
                  <Input className="font-mono" value={editing.name} onChange={(e) => setEditing({ ...editing, name: e.target.value })} />
                </Field>
              )}
              <CodeEditor
                language={type === "caddy" ? "text" : "yaml"}
                value={editing.content}
                onChange={(content) => setEditing({ ...editing, content })}
                readOnly={!canManage}
                minHeight="320px"
              />
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)}>
              Fermer
            </Button>
            {canManage && (
              <Button loading={save.isPending} onClick={() => editing && save.mutate(editing)}>
                Enregistrer
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Supprimer ${deleting} ?`}
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </div>
  )
}

function Certificates({ team, server, canManage }: { team: string; server: string; canManage: boolean }) {
  const base = teamPath(team, `/servers/${server}/proxy/certificates`)
  const key = ["team", team, "server", server, "certificates"]
  const certs = useQuery({ queryKey: key, queryFn: () => get<Certificate[]>(base) })
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState({ name: "", certificate: "", private_key: "" })
  const add = useApiMutation((v: typeof form) => post(base, v), {
    success: "Certificat ajouté",
    invalidate: [key],
    onSuccess: () => {
      setAdding(false)
      setForm({ name: "", certificate: "", private_key: "" })
    },
  })
  const remove = useApiMutation((id: string) => del(`${base}/${id}`), { success: "Certificat supprimé", invalidate: [key] })
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">Certificats personnalisés (sinon Let's Encrypt automatique).</p>
        {canManage && (
          <Button size="sm" onClick={() => setAdding(true)}>
            <PlusIcon /> Ajouter
          </Button>
        )}
      </div>
      {!certs.data?.length ? (
        <p className="py-6 text-center text-sm text-muted-foreground">Aucun certificat personnalisé.</p>
      ) : (
        <Card>
          <ul className="divide-y">
            {certs.data.map((c) => (
              <li key={c.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
                <span className="font-medium">{c.name}</span>
                <span className="truncate text-xs text-muted-foreground">{c.domains.join(", ")}</span>
                <span className="ml-auto shrink-0 text-xs text-muted-foreground">expire le {dateTime(c.expires_at)}</span>
                {canManage && (
                  <Button variant="ghost" size="icon-sm" onClick={() => remove.mutate(c.id)} aria-label="Supprimer">
                    <Trash2Icon />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </Card>
      )}
      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Certificat personnalisé</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <Field label="Nom">
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </Field>
            <Field label="Certificat (PEM, chaîne possible)">
              <Textarea rows={6} className="font-mono text-xs" value={form.certificate} onChange={(e) => setForm({ ...form, certificate: e.target.value })} />
            </Field>
            <Field label="Clé privée (PEM)">
              <Textarea rows={5} className="font-mono text-xs" value={form.private_key} onChange={(e) => setForm({ ...form, private_key: e.target.value })} />
            </Field>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAdding(false)}>
              Annuler
            </Button>
            <Button loading={add.isPending} onClick={() => add.mutate(form)}>
              Ajouter
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ProxySettings({ team, server, proxy, canManage }: { team: string; server: string; proxy: ProxyState; canManage: boolean }) {
  const base = teamPath(team, `/servers/${server}/proxy`)
  const [type, setType] = useState(proxy.type)
  const [email, setEmail] = useState(proxy.acme_email)
  const [compose, setCompose] = useState(proxy.is_custom ? proxy.compose : proxy.default_compose)
  const save = useApiMutation((v: Record<string, unknown>) => patch(base, v), {
    success: "Proxy mis à jour (redémarrez-le pour appliquer)",
    invalidate: [["team", team, "server", server, "proxy"], ["team", team, "server", server]],
  })
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Type et certificats</CardTitle>
          <CardDescription>Changer de type exige un proxy arrêté.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field label="Type de proxy">
            <Select value={type} onValueChange={(v) => setType(v as ProxyState["type"])} disabled={!canManage}>
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
          <Field label="E-mail Let's Encrypt" hint="Avertissements d'expiration des certificats.">
            <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} disabled={!canManage} />
          </Field>
          {canManage && (
            <Button loading={save.isPending} onClick={() => save.mutate({ type, acme_email: email })}>
              Enregistrer
            </Button>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>docker-compose du proxy</CardTitle>
          <CardDescription>Laisser la configuration générée sauf besoin particulier.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <CodeEditor language="yaml" value={compose} onChange={setCompose} readOnly={!canManage} minHeight="260px" />
          {canManage && (
            <div className="flex gap-2">
              <Button loading={save.isPending} onClick={() => save.mutate({ compose })}>
                Enregistrer
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setCompose(proxy.default_compose)
                  save.mutate({ compose: "" })
                }}
              >
                Revenir à la configuration générée
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
