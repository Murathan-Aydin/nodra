import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { CheckCircle2Icon, RefreshCwIcon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { SwitchRow } from "@/features/resources/operations"
import { useDraft } from "@/features/resources/use-patch"
import { SaveButton } from "@/features/applications/general"
import { api, get } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import type { PrivateKey, Server } from "@/lib/types"
import { serverState } from "./server"
import { TaskButton, useServerTask } from "./run-task"

/** Enregistrement partiel d'un serveur. */
export function useServerPatch(team: string, id: string, message = "Serveur enregistré") {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (fields: Record<string, unknown>) => api(`/teams/${team}/servers/${id}`, { method: "PATCH", json: fields }),
    onSuccess: () => {
      toast.success(message)
      void qc.invalidateQueries({ queryKey: ["team", team, "server", id] })
      void qc.invalidateQueries({ queryKey: ["team", team, "servers"] })
    },
    onError: (e) => toast.error(e.message),
  })
}

const settingStr = (s: Server, k: string, d = "") => String(s.settings?.[k] ?? d)

/** Onglet « Général » d'un serveur : connexion, état, validation et réglages de déploiement. */
export function ServerGeneral({ team, server }: { team: string; server: Server }) {
  const state = serverState(server)
  const validate = useServerTask(team, server.id)
  const install = useServerTask(team, server.id)
  const { draft: d, set, changed, dirty } = useDraft({ name: server.name, description: server.description, ip: server.ip, port: String(server.port), user: server.user })
  const save = useServerPatch(team, server.id)
  const reconnect = ["ip", "port", "user"].some((k) => k in changed)
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Serveur"
        description={server.is_localhost ? "Serveur de l'instance (cette machine)." : undefined}
        actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ ...changed, ...(changed.port ? { port: Number(changed.port) } : {}) })} />}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Nom *">
            <Input value={d.name} onChange={(e) => set("name", e.target.value)} />
          </Field>
          <Field label="Description">
            <Input value={d.description} onChange={(e) => set("description", e.target.value)} />
          </Field>
          <Field label="Adresse IP ou domaine">
            <Input value={d.ip} onChange={(e) => set("ip", e.target.value)} className="font-mono" disabled={server.is_localhost} />
          </Field>
          <div className="grid grid-cols-2 gap-4">
            <Field label="Port SSH">
              <Input value={d.port} onChange={(e) => set("port", e.target.value.replace(/\D/g, ""))} className="font-mono" disabled={server.is_localhost} />
            </Field>
            <Field label="Utilisateur">
              <Input value={d.user} onChange={(e) => set("user", e.target.value)} className="font-mono" disabled={server.is_localhost} />
            </Field>
          </div>
        </div>
        {reconnect && <p className="mt-3 text-xs text-warning">Changer l'adresse, le port ou l'utilisateur repasse le serveur en « non validé ».</p>}
      </SettingsCard>

      <SettingsCard
        title="État"
        actions={
          <>
            <TaskButton label="Valider le serveur" icon={<CheckCircle2Icon />} pending={validate.run.isPending} onClick={() => validate.run.mutate({ path: "/validate" })} />
            {!server.docker_version && <TaskButton label="Installer Docker" icon={<RefreshCwIcon />} pending={install.run.isPending} onClick={() => install.run.mutate({ path: "/install-docker" })} />}
          </>
        }
      >
        <div className="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2">
          <Info label="Statut" value={<Pill label={state.label} dot={state.dot} />} />
          <Info label="Dernier contact" value={relativeTime(server.last_seen_at)} />
          <Info label="Système" value={server.os_id ? `${server.os_id} ${server.os_version}` : "—"} />
          <Info label="Docker" value={server.docker_version ? `${server.docker_version}${server.compose_version ? ` · Compose ${server.compose_version}` : ""}` : "Non installé"} />
          <Info label="Validé le" value={dateTime(server.validated_at)} />
          <Info label="Dernier nettoyage" value={relativeTime(server.last_cleanup_at)} />
        </div>
        {server.validation_error && <p className="mt-3 text-sm text-destructive">{server.validation_error}</p>}
        {validate.output && <div className="mt-4">{validate.output}</div>}
        {install.output && <div className="mt-4">{install.output}</div>}
      </SettingsCard>

      <DeploySettings team={team} server={server} />
    </div>
  )
}

function Info({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border py-2 last:border-b-0">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right">{value}</span>
    </div>
  )
}

function DeploySettings({ team, server }: { team: string; server: Server }) {
  const { draft: d, set, changed, dirty } = useDraft({
    wildcard_domain: settingStr(server, "wildcard_domain"),
    timezone: settingStr(server, "timezone", "UTC"),
    concurrent_builds: settingStr(server, "concurrent_builds", "2"),
    deployment_timeout: settingStr(server, "deployment_timeout", "3600"),
    connection_timeout: settingStr(server, "connection_timeout", "10"),
    is_build_server: Boolean(server.settings?.is_build_server),
  })
  const save = useServerPatch(team, server.id)
  const NUM = new Set(["concurrent_builds", "deployment_timeout", "connection_timeout"])
  return (
    <SettingsCard
      title="Déploiements"
      actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ settings: Object.fromEntries(Object.entries(changed).map(([k, v]) => [k, NUM.has(k) ? Number(v) : v])) })} />}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Domaine générique" hint="Ex. « https://apps.exemple.fr » : les nouvelles ressources reçoivent un sous-domaine (sinon sslip.io).">
          <Input value={d.wildcard_domain} onChange={(e) => set("wildcard_domain", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Fuseau horaire" hint="Pour les tâches planifiées et les sauvegardes.">
          <Input value={d.timezone} onChange={(e) => set("timezone", e.target.value)} className="font-mono" placeholder="Europe/Paris" />
        </Field>
        <Field label="Builds simultanés">
          <Input value={d.concurrent_builds} onChange={(e) => set("concurrent_builds", e.target.value.replace(/\D/g, ""))} />
        </Field>
        <Field label="Durée max. d'un déploiement (s)">
          <Input value={d.deployment_timeout} onChange={(e) => set("deployment_timeout", e.target.value.replace(/\D/g, ""))} />
        </Field>
        <Field label="Délai de connexion SSH (s)">
          <Input value={d.connection_timeout} onChange={(e) => set("connection_timeout", e.target.value.replace(/\D/g, ""))} />
        </Field>
      </div>
      <SwitchRow label="Serveur de build" hint="Construit les images sans y déployer de ressources." checked={d.is_build_server} onChange={(v) => set("is_build_server", v)} />
    </SettingsCard>
  )
}

/** Onglet « Avancé » : métriques. */
export function ServerAdvanced({ team, server }: { team: string; server: Server }) {
  const { draft: d, set, changed, dirty } = useDraft({ metrics_enabled: server.settings?.metrics_enabled !== false, metrics_retention_days: settingStr(server, "metrics_retention_days", "7") })
  const save = useServerPatch(team, server.id)
  return (
    <SettingsCard
      title="Métriques"
      description="Relevées chaque minute (processeur, mémoire, disque, charge)."
      actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ settings: { ...changed, ...(changed.metrics_retention_days ? { metrics_retention_days: Number(changed.metrics_retention_days) } : {}) } })} />}
    >
      <SwitchRow label="Collecter les métriques" checked={d.metrics_enabled} onChange={(v) => set("metrics_enabled", v)} />
      <Field label="Conservation (jours)" className="max-w-xs">
        <Input value={d.metrics_retention_days} onChange={(e) => set("metrics_retention_days", e.target.value.replace(/\D/g, ""))} />
      </Field>
    </SettingsCard>
  )
}

/** Onglet « Clé privée » : clé SSH utilisée et clé d'hôte enregistrée. */
export function ServerPrivateKey({ team, server }: { team: string; server: Server }) {
  const qc = useQueryClient()
  const keys = useQuery({ queryKey: ["team", team, "private-keys"], queryFn: () => get<PrivateKey[]>(`/teams/${team}/private-keys`) }).data ?? []
  const [key, setKey] = useState(server.private_key_id ?? "")
  const [resetting, setResetting] = useState(false)
  const save = useServerPatch(team, server.id, "Clé enregistrée : revalidez le serveur")
  const reset = useMutation({
    mutationFn: () => api(`/teams/${team}/servers/${server.id}/host-key/reset`, { method: "POST", json: {} }),
    onSuccess: () => {
      setResetting(false)
      toast.success("Clé d'hôte oubliée : revalidez le serveur")
      void qc.invalidateQueries({ queryKey: ["team", team, "server", server.id] })
    },
    onError: (e) => toast.error(e.message),
  })
  const current = keys.find((k) => k.id === key)
  return (
    <div className="space-y-6">
      <SettingsCard title="Clé privée" description="Clé SSH utilisée pour se connecter au serveur." actions={<SaveButton dirty={key !== (server.private_key_id ?? "")} pending={save.isPending} onClick={() => save.mutate({ private_key_id: key })} />}>
        <Field label="Clé" className="max-w-md">
          <Select value={key} onValueChange={setKey}>
            <SelectTrigger>
              <SelectValue placeholder="Choisir une clé" />
            </SelectTrigger>
            <SelectContent>
              {keys.map((k) => (
                <SelectItem key={k.id} value={k.id}>
                  {k.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {current && (
          <Field label="Clé publique (à placer dans ~/.ssh/authorized_keys)" className="mt-4">
            <CopyInput value={current.public_key} />
          </Field>
        )}
      </SettingsCard>
      <SettingsCard
        title="Clé d'hôte"
        description="Enregistrée à la première connexion ; protège contre l'usurpation du serveur."
        actions={
          <Button size="sm" variant="outline" onClick={() => setResetting(true)} disabled={!server.host_key}>
            Oublier la clé d'hôte
          </Button>
        }
      >
        {server.host_key ? <Textarea value={server.host_key} readOnly rows={3} className="font-mono" /> : <p className="text-sm text-muted-foreground">Aucune clé enregistrée.</p>}
      </SettingsCard>
      <ConfirmDialog open={resetting} onOpenChange={setResetting} title="Oublier la clé d'hôte ?" description="À faire seulement si le serveur a été réinstallé. Il faudra le revalider." actionLabel="Oublier" loading={reset.isPending} onConfirm={() => reset.mutate()} />
    </div>
  )
}

/** Onglet « Certificat CA » : autorité qui signe les certificats TLS des bases. */
export function ServerCA({ team, server }: { team: string; server: Server }) {
  const ca = useQuery({ queryKey: ["team", team, "server", server.id, "ca"], queryFn: () => get<{ certificate: string }>(`/teams/${team}/servers/${server.id}/ca-certificate`) })
  return (
    <SettingsCard title="Certificat de l'autorité" description="Signe les certificats des bases de données en TLS. À fournir aux clients en mode verify-ca / verify-full.">
      {ca.data?.certificate ? <Textarea value={ca.data.certificate} readOnly rows={14} className="font-mono" /> : <p className="text-sm text-muted-foreground">{ca.isPending ? "Chargement…" : "Aucune autorité pour l'instant (créée au premier démarrage d'une base en TLS)."}</p>}
    </SettingsCard>
  )
}

/** Onglet « Cloudflare Tunnel ». */
export function ServerCloudflare({ team, server }: { team: string; server: Server }) {
  const active = Boolean((server.settings?.cloudflare_tunnel as { enabled?: boolean } | undefined)?.enabled ?? server.settings?.cloudflare_tunnel)
  const [token, setToken] = useState("")
  const start = useServerTask(team, server.id)
  const stop = useServerTask(team, server.id)
  return (
    <SettingsCard
      title="Cloudflare Tunnel"
      description="Rend le serveur accessible sans ouvrir de port : le trafic passe par un tunnel Cloudflare. Les domaines sont alors considérés corrects par la vérification DNS."
      actions={active ? <TaskButton label="Arrêter le tunnel" pending={stop.run.isPending} onClick={() => stop.run.mutate({ path: "/cloudflared", method: "DELETE" })} /> : undefined}
    >
      <p className="mb-4 text-sm">
        État : <strong>{active ? "actif" : "inactif"}</strong>
      </p>
      <div className="flex max-w-xl gap-2">
        <Input value={token} onChange={(e) => setToken(e.target.value)} type="password" placeholder="Jeton du tunnel (Cloudflare Zero Trust)" className="font-mono" />
        <Button size="sm" disabled={!token} loading={start.run.isPending} onClick={() => start.run.mutate({ path: "/cloudflared", method: "PUT", json: { token } })}>
          {active ? "Remplacer" : "Démarrer"}
        </Button>
      </div>
      {start.output && <div className="mt-4">{start.output}</div>}
      {stop.output && <div className="mt-4">{stop.output}</div>}
    </SettingsCard>
  )
}
