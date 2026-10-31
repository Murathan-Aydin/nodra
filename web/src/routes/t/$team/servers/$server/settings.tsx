import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import { DownloadIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { CodeEditor } from "@/components/code-editor"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { TaskDialog } from "@/components/task-output"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { del, get, patch, post, put, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Server, ServerSettings, ServerTask } from "@/lib/types"
import { keysQuery, serverQuery } from "@/features/servers/queries"

export const Route = createFileRoute("/t/$team/servers/$server/settings")({ component: ServerSettingsPage })

function ServerSettingsPage() {
  const { team, server } = Route.useParams()
  const { data } = useQuery(serverQuery(team, server))
  const canUpdate = usePermission(team, "server.update", { type: "server", id: server })
  const canDelete = usePermission(team, "server.delete", { type: "server", id: server })
  const [task, setTask] = useState<ServerTask | null>(null)
  if (!data) return null
  return (
    <div className="space-y-6">
      <General team={team} server={data.server} disabled={!canUpdate} />
      <SettingsForm team={team} server={data.server} disabled={!canUpdate} />
      <Destinations team={team} server={server} destinations={data.destinations} disabled={!canUpdate} onTask={setTask} />
      <CloudflareTunnel team={team} server={data.server} disabled={!canUpdate} onTask={setTask} />
      <LogDrain team={team} server={server} disabled={!canUpdate} onTask={setTask} />
      <CACertificate team={team} server={server} />
      <DangerZone team={team} server={data.server} canUpdate={canUpdate} canDelete={canDelete} />
      <TaskDialog team={team} task={task} onOpenChange={(o) => !o && setTask(null)} />
    </div>
  )
}

function Section({ title, description, children, footer }: { title: string; description?: ReactNode; children: ReactNode; footer?: ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent>{children}</CardContent>
      {footer && <CardFooter className="justify-end">{footer}</CardFooter>}
    </Card>
  )
}

function useServerPatch(team: string, server: string) {
  return useApiMutation((v: Record<string, unknown>) => patch<Server>(teamPath(team, `/servers/${server}`), v), {
    success: "Serveur mis à jour",
    invalidate: [["team", team, "server", server], ["team", team, "servers"]],
  })
}

function General({ team, server, disabled }: { team: string; server: Server; disabled: boolean }) {
  const keys = useQuery(keysQuery(team))
  const [form, setForm] = useState({
    name: server.name,
    description: server.description,
    ip: server.ip,
    port: String(server.port),
    user: server.user,
    private_key_id: server.private_key_id,
  })
  const save = useServerPatch(team, server.id)
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value })
  return (
    <Section
      title="Connexion"
      description="Changer l'adresse, le port, l'utilisateur ou la clé impose une nouvelle validation."
      footer={
        !disabled && (
          <Button loading={save.isPending} onClick={() => save.mutate({ ...form, port: Number(form.port) || 22 })}>
            Enregistrer
          </Button>
        )
      }
    >
      <div className="grid gap-4 sm:grid-cols-6">
        <Field label="Nom" className="sm:col-span-3">
          <Input value={form.name} onChange={set("name")} disabled={disabled} />
        </Field>
        <Field label="Description" className="sm:col-span-3">
          <Input value={form.description} onChange={set("description")} disabled={disabled} />
        </Field>
        <Field label="Adresse" className="sm:col-span-3">
          <Input value={form.ip} onChange={set("ip")} disabled={disabled} />
        </Field>
        <Field label="Port" className="sm:col-span-1">
          <Input value={form.port} onChange={set("port")} disabled={disabled} />
        </Field>
        <Field label="Utilisateur" className="sm:col-span-2">
          <Input value={form.user} onChange={set("user")} disabled={disabled} />
        </Field>
        <Field label="Clé SSH" className="sm:col-span-3">
          <Select value={form.private_key_id} onValueChange={(v) => setForm({ ...form, private_key_id: v })} disabled={disabled}>
            <SelectTrigger>
              <SelectValue />
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
      </div>
    </Section>
  )
}

function SettingsForm({ team, server, disabled }: { team: string; server: Server; disabled: boolean }) {
  const [s, setS] = useState<ServerSettings>(server.settings)
  const save = useServerPatch(team, server.id)
  const num = (k: keyof ServerSettings) => (e: React.ChangeEvent<HTMLInputElement>) => setS({ ...s, [k]: Number(e.target.value) })
  const str = (k: keyof ServerSettings) => (e: React.ChangeEvent<HTMLInputElement>) => setS({ ...s, [k]: e.target.value })
  const sw = (k: keyof ServerSettings, label: string, hint?: string) => (
    <Label className="flex items-start justify-between gap-4 font-normal">
      <span>
        <span className="block text-sm font-medium">{label}</span>
        {hint && <span className="block text-xs text-muted-foreground">{hint}</span>}
      </span>
      <Switch checked={!!s[k]} onCheckedChange={(v) => setS({ ...s, [k]: v })} disabled={disabled} />
    </Label>
  )
  return (
    <Section
      title="Réglages"
      footer={
        !disabled && (
          <Button loading={save.isPending} onClick={() => save.mutate({ settings: s })}>
            Enregistrer
          </Button>
        )
      }
    >
      <div className="grid gap-x-8 gap-y-4 md:grid-cols-2">
        <Field label="Builds simultanés">
          <Input type="number" min={1} value={s.concurrent_builds} onChange={num("concurrent_builds")} disabled={disabled} />
        </Field>
        <Field label="Délai max. d'un déploiement (s)">
          <Input type="number" min={60} value={s.deployment_timeout} onChange={num("deployment_timeout")} disabled={disabled} />
        </Field>
        <Field label="Domaine joker" hint="ex. https://apps.example.com : domaines générés en sous-domaines.">
          <Input value={s.wildcard_domain} onChange={str("wildcard_domain")} disabled={disabled} />
        </Field>
        <Field label="Fuseau horaire" hint="Planifications (sauvegardes, tâches, nettoyage).">
          <Input value={s.timezone} onChange={str("timezone")} disabled={disabled} />
        </Field>
        <Field label="Nettoyage Docker (cron)" hint="ex. 0 0 * * * ou @daily">
          <Input className="font-mono" value={s.docker_cleanup_frequency} onChange={str("docker_cleanup_frequency")} disabled={disabled} />
        </Field>
        <Field label="Seuil de nettoyage (% disque)">
          <Input type="number" min={1} max={100} value={s.docker_cleanup_threshold} onChange={num("docker_cleanup_threshold")} disabled={disabled} />
        </Field>
        <Field label="Rétention des métriques (jours)">
          <Input type="number" min={1} value={s.metrics_retention_days} onChange={num("metrics_retention_days")} disabled={disabled} />
        </Field>
        <Field label="Délai de connexion SSH (s)">
          <Input type="number" min={1} value={s.connection_timeout} onChange={num("connection_timeout")} disabled={disabled} />
        </Field>
        {sw("metrics_enabled", "Métriques", "CPU, mémoire, disque relevés chaque minute.")}
        {sw("delete_unused_volumes", "Supprimer les volumes inutilisés au nettoyage")}
        {sw("delete_unused_networks", "Supprimer les réseaux inutilisés au nettoyage")}
        {sw("is_build_server", "Serveur de build")}
      </div>
    </Section>
  )
}

function Destinations({
  team,
  server,
  destinations,
  disabled,
  onTask,
}: {
  team: string
  server: string
  destinations: { id: string; name: string; network: string; is_default: boolean }[]
  disabled: boolean
  onTask: (t: ServerTask) => void
}) {
  const [network, setNetwork] = useState("")
  const invalidate = [["team", team, "server", server]]
  const create = useApiMutation((n: string) => post<ServerTask>(teamPath(team, `/servers/${server}/destinations`), { network: n }), {
    invalidate,
    onSuccess: (t) => {
      setNetwork("")
      if (t?.id) onTask(t)
    },
  })
  const remove = useApiMutation((id: string) => del<ServerTask>(teamPath(team, `/servers/${server}/destinations/${id}`)), {
    invalidate,
    onSuccess: (t) => t?.id && onTask(t),
  })
  return (
    <Section title="Destinations" description="Réseaux Docker où les ressources sont déployées.">
      <ul className="mb-4 divide-y rounded-md border">
        {destinations.map((d) => (
          <li key={d.id} className="flex items-center gap-2 px-3 py-2 text-sm">
            <span className="font-mono">{d.network}</span>
            {d.is_default && <Badge variant="secondary">par défaut</Badge>}
            {!disabled && !d.is_default && (
              <Button variant="ghost" size="icon-sm" className="ml-auto" onClick={() => remove.mutate(d.id)} aria-label="Supprimer">
                <Trash2Icon />
              </Button>
            )}
          </li>
        ))}
      </ul>
      {!disabled && (
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (network.trim()) create.mutate(network.trim())
          }}
        >
          <Input className="max-w-xs font-mono" placeholder="nom-du-reseau" value={network} onChange={(e) => setNetwork(e.target.value)} />
          <Button type="submit" variant="outline" loading={create.isPending}>
            <PlusIcon /> Ajouter
          </Button>
        </form>
      )}
    </Section>
  )
}

function CloudflareTunnel({ team, server, disabled, onTask }: { team: string; server: Server; disabled: boolean; onTask: (t: ServerTask) => void }) {
  const [token, setToken] = useState("")
  const path = teamPath(team, `/servers/${server.id}/cloudflared`)
  const invalidate = [["team", team, "server", server.id]]
  const start = useApiMutation((t: string) => put<ServerTask>(path, { token: t }), { invalidate, onSuccess: (t) => (setToken(""), onTask(t)) })
  const stop = useApiMutation(() => del<ServerTask>(path), { invalidate, onSuccess: onTask })
  return (
    <Section
      title="Cloudflare Tunnel"
      description="Expose le serveur via Cloudflare sans ouvrir de ports (le jeton est chiffré)."
    >
      <div className="flex flex-wrap items-center gap-2">
        {server.settings.cloudflare_tunnel && <Badge variant="success">Actif</Badge>}
        {!disabled && (
          <>
            <Input className="max-w-md font-mono" type="password" placeholder="Jeton du tunnel" value={token} onChange={(e) => setToken(e.target.value)} />
            <Button variant="outline" disabled={!token} loading={start.isPending} onClick={() => start.mutate(token)}>
              Démarrer
            </Button>
            {server.settings.cloudflare_tunnel && (
              <Button variant="outline" loading={stop.isPending} onClick={() => stop.mutate(undefined)}>
                Arrêter
              </Button>
            )}
          </>
        )}
      </div>
    </Section>
  )
}

interface DrainState {
  enabled: boolean
  type?: "newrelic" | "axiom" | "custom"
  config?: Record<string, string>
}

function LogDrain({ team, server, disabled, onTask }: { team: string; server: string; disabled: boolean; onTask: (t: ServerTask) => void }) {
  const path = teamPath(team, `/servers/${server}/log-drain`)
  const key = ["team", team, "server", server, "log-drain"]
  const drain = useQuery({ queryKey: key, queryFn: () => get<DrainState>(path) })
  const [type, setType] = useState<string>("")
  const [cfg, setCfg] = useState<Record<string, string>>({})
  const current = type || drain.data?.type || "newrelic"
  const values = { ...(drain.data?.config ?? {}), ...cfg }
  const save = useApiMutation(() => put<{ task: ServerTask }>(path, { type: current, ...values }), {
    invalidate: [key],
    onSuccess: (r) => onTask(r.task),
  })
  const remove = useApiMutation(() => del<ServerTask>(path), { invalidate: [key], onSuccess: onTask })
  const field = (k: string, label: string, opts: { secret?: boolean; hint?: string; placeholder?: string } = {}) => (
    <Field label={label} hint={opts.hint}>
      <Input
        type={opts.secret ? "password" : "text"}
        className="font-mono"
        placeholder={opts.placeholder}
        value={values[k] ?? ""}
        onChange={(e) => setCfg({ ...cfg, [k]: e.target.value })}
        disabled={disabled}
      />
    </Field>
  )
  return (
    <Section
      title="Log drain"
      description="Envoie les logs des ressources qui l'activent vers New Relic, Axiom ou une sortie Fluent Bit."
      footer={
        !disabled && (
          <div className="flex gap-2">
            {drain.data?.enabled && (
              <Button variant="outline" loading={remove.isPending} onClick={() => remove.mutate(undefined)}>
                Désactiver
              </Button>
            )}
            <Button loading={save.isPending} onClick={() => save.mutate(undefined)}>
              {drain.data?.enabled ? "Mettre à jour" : "Activer"}
            </Button>
          </div>
        )
      }
    >
      <div className="space-y-4">
        <div className="flex items-center gap-3">
          {drain.data?.enabled ? <Badge variant="success">Actif ({drain.data.type})</Badge> : <Badge variant="secondary">Inactif</Badge>}
          <Select value={current} onValueChange={setType} disabled={disabled}>
            <SelectTrigger className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="newrelic">New Relic</SelectItem>
              <SelectItem value="axiom">Axiom</SelectItem>
              <SelectItem value="custom">Personnalisé (Fluent Bit)</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {current === "newrelic" && (
          <div className="grid gap-4 md:grid-cols-2">
            {field("license_key", "Clé de licence", { secret: true })}
            {field("base_uri", "URL d'ingestion", { placeholder: "https://log-api.eu.newrelic.com/log/v1" })}
          </div>
        )}
        {current === "axiom" && (
          <div className="grid gap-4 md:grid-cols-3">
            {field("axiom_dataset", "Dataset")}
            {field("axiom_api_key", "Clé d'API", { secret: true })}
            {field("axiom_domain", "Domaine", { placeholder: "api.axiom.co" })}
          </div>
        )}
        {current === "custom" && (
          <div className="space-y-3">
            <Field label="fluent-bit.conf" hint="@INCLUDE, @SET et les extensions exec/lua sont refusés.">
              <CodeEditor value={values.custom_config ?? ""} onChange={(v) => setCfg({ ...cfg, custom_config: v })} readOnly={disabled} minHeight="200px" />
            </Field>
            <Field label="parsers.conf (facultatif)">
              <CodeEditor value={values.custom_parsers ?? ""} onChange={(v) => setCfg({ ...cfg, custom_parsers: v })} readOnly={disabled} minHeight="100px" />
            </Field>
          </div>
        )}
      </div>
    </Section>
  )
}

function CACertificate({ team, server }: { team: string; server: string }) {
  const download = useApiMutation(() => get<{ certificate: string }>(teamPath(team, `/servers/${server}/ca-certificate`)), {
    onSuccess: ({ certificate }) => {
      const url = URL.createObjectURL(new Blob([certificate], { type: "application/x-pem-file" }))
      const a = Object.assign(document.createElement("a"), { href: url, download: "nodra-ca.crt" })
      a.click()
      URL.revokeObjectURL(url)
    },
  })
  return (
    <Section title="Autorité de certification" description="Signe les certificats TLS des bases de données de ce serveur : à fournir aux clients (verify-ca).">
      <Button variant="outline" loading={download.isPending} onClick={() => download.mutate(undefined)}>
        <DownloadIcon /> Télécharger le certificat
      </Button>
    </Section>
  )
}

function DangerZone({ team, server, canUpdate, canDelete }: { team: string; server: Server; canUpdate: boolean; canDelete: boolean }) {
  const navigate = useNavigate()
  const [confirm, setConfirm] = useState<"reset" | "delete" | null>(null)
  const reset = useApiMutation(() => post(teamPath(team, `/servers/${server.id}/host-key/reset`)), {
    success: "Clé d'hôte oubliée : revalidez le serveur",
    invalidate: [["team", team, "server", server.id]],
    onSuccess: () => setConfirm(null),
  })
  const remove = useApiMutation(() => del(teamPath(team, `/servers/${server.id}`)), {
    success: (r) => (r ? undefined : "Serveur supprimé"),
    invalidate: [["team", team, "servers"]],
    onSuccess: (r) => {
      setConfirm(null)
      if (!r) void navigate({ to: "/t/$team/servers", params: { team } })
    },
  })
  if (!canUpdate && !canDelete) return null
  return (
    <Card className="border-destructive/40">
      <CardHeader>
        <CardTitle className="text-destructive">Zone de danger</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {canUpdate && (
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium">Réinitialiser la clé d'hôte</p>
              <p className="text-xs text-muted-foreground">Après une réinstallation du serveur (la clé SSH de l'hôte a changé).</p>
            </div>
            <Button variant="outline" onClick={() => setConfirm("reset")}>
              Réinitialiser
            </Button>
          </div>
        )}
        {canDelete && (
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium">Supprimer le serveur</p>
              <p className="text-xs text-muted-foreground">Impossible tant que des ressources y sont déployées. Peut exiger une approbation.</p>
            </div>
            <Button variant="destructive" onClick={() => setConfirm("delete")}>
              Supprimer
            </Button>
          </div>
        )}
      </CardContent>
      <ConfirmDialog
        open={confirm === "reset"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Réinitialiser la clé d'hôte ?"
        description="Le serveur devra être revalidé ; la nouvelle clé sera acceptée à la prochaine connexion."
        loading={reset.isPending}
        onConfirm={() => reset.mutate(undefined)}
      />
      <ConfirmDialog
        open={confirm === "delete"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Supprimer le serveur ?"
        description="Nodra cesse de le gérer ; rien n'est effacé sur la machine."
        confirmText={server.name}
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => remove.mutate(undefined)}
      />
    </Card>
  )
}
