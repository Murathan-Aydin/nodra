import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { LogOutIcon, RefreshCwIcon, SendIcon, ShieldIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { SaveButton } from "@/features/applications/general"
import { SwitchRow } from "@/features/resources/operations"
import { useDraft } from "@/features/resources/use-patch"
import { api, get, post } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { meQuery } from "@/lib/queries"
import { AuditLog } from "@/features/team/audit"

type Settings = {
  fqdn: string
  instance_name: string
  public_ipv4: string
  public_ipv6: string
  timezone: string
  registration_enabled: boolean
  auto_update_enabled: boolean
  auto_update_frequency: string
  update_check_frequency: string
  dns_validation_enabled: boolean
  dns_servers: string[] | null
  api_enabled: boolean
  api_allowed_ips: string[] | null
  mcp_enabled: boolean
  latest_version: string
  update_checked_at: string | null
}
type AdminSettings = { settings: Settings; version: string; update_available: boolean }

const settingsKey = ["admin", "settings"]
const useSettings = () => useQuery({ queryKey: settingsKey, queryFn: () => get<AdminSettings>("/admin/settings") })

function usePatchSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (fields: Partial<Settings>) => api<AdminSettings>("/admin/settings", { method: "PATCH", json: fields }),
    onSuccess: (r) => {
      qc.setQueryData(settingsKey, r)
      void qc.invalidateQueries({ queryKey: ["instance"] })
      toast.success("Réglages enregistrés")
    },
    onError: (e) => toast.error(e.message),
  })
}

const list = (s: string) => s.split(/[\s,]+/).filter(Boolean)

/** Réglages › Général (settings.index du fork) : nom, domaine, adresses, fuseau, inscriptions. */
export function AdminGeneralPage() {
  const s = useSettings().data?.settings
  return (
    <>
      <PageHeader title="Réglages de l'instance" />
      {s && <GeneralForm key={JSON.stringify(s)} s={s} />}
    </>
  )
}

function GeneralForm({ s }: { s: Settings }) {
  const { draft: d, set, changed, dirty } = useDraft({ instance_name: s.instance_name, fqdn: s.fqdn, public_ipv4: s.public_ipv4, public_ipv6: s.public_ipv6, timezone: s.timezone, registration_enabled: s.registration_enabled })
  const save = usePatchSettings()
  return (
    <SettingsCard title="Instance" actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(changed)} />}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Nom">
          <Input value={d.instance_name} onChange={(e) => set("instance_name", e.target.value)} />
        </Field>
        <Field label="Domaine de l'instance" hint="Liens des e-mails, GitHub App ; l'interface est servie sur ce domaine par le proxy (HTTPS automatique).">
          <Input value={d.fqdn} onChange={(e) => set("fqdn", e.target.value)} className="font-mono" placeholder="https://nodra.exemple.fr" />
        </Field>
        <Field label="IPv4 publique">
          <Input value={d.public_ipv4} onChange={(e) => set("public_ipv4", e.target.value)} className="font-mono" />
        </Field>
        <Field label="IPv6 publique">
          <Input value={d.public_ipv6} onChange={(e) => set("public_ipv6", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Fuseau horaire">
          <Input value={d.timezone} onChange={(e) => set("timezone", e.target.value)} className="font-mono" placeholder="Europe/Paris" />
        </Field>
      </div>
      <SwitchRow label="Inscriptions ouvertes" hint="Sinon, seuls les invités peuvent créer un compte." checked={d.registration_enabled} onChange={(v) => set("registration_enabled", v)} />
    </SettingsCard>
  )
}

/** Réglages › Avancé : DNS, API, MCP. */
export function AdminAdvancedPage() {
  const s = useSettings().data?.settings
  return (
    <>
      <PageHeader title="Réglages avancés" />
      {s && <AdvancedForm key={JSON.stringify(s)} s={s} />}
    </>
  )
}

function AdvancedForm({ s }: { s: Settings }) {
  const { draft: d, set, dirty } = useDraft({
    dns_validation_enabled: s.dns_validation_enabled,
    dns_servers: (s.dns_servers ?? []).join(", "),
    api_enabled: s.api_enabled,
    api_allowed_ips: (s.api_allowed_ips ?? []).join(", "),
    mcp_enabled: s.mcp_enabled,
  })
  const save = usePatchSettings()
  return (
    <div className="space-y-6">
      <SettingsCard
        title="DNS et API"
        actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ ...d, dns_servers: list(d.dns_servers), api_allowed_ips: list(d.api_allowed_ips) })} />}
      >
        <SwitchRow label="Vérifier les DNS des domaines" checked={d.dns_validation_enabled} onChange={(v) => set("dns_validation_enabled", v)} />
        <Field label="Serveurs DNS" hint="IP ou IP:port, 5 max. ; vide = résolveur du système.">
          <Input value={d.dns_servers} onChange={(e) => set("dns_servers", e.target.value)} className="font-mono" placeholder="1.1.1.1, 8.8.8.8" />
        </Field>
        <SwitchRow label="API par jetons" hint="Désactivée : toute requête par jeton est refusée." checked={d.api_enabled} onChange={(v) => set("api_enabled", v)} />
        <Field label="IP autorisées pour les jetons" hint="IP ou plages CIDR ; vide = toutes.">
          <Input value={d.api_allowed_ips} onChange={(e) => set("api_allowed_ips", e.target.value)} className="font-mono" />
        </Field>
        <SwitchRow label="Serveur MCP" hint="Expose les outils Nodra aux assistants IA sur /api/v1/mcp (avec un jeton API)." checked={d.mcp_enabled} onChange={(v) => set("mcp_enabled", v)} />
      </SettingsCard>
      {d.mcp_enabled && (
        <SettingsCard title="Adresse MCP">
          <CopyInput value={`${location.origin}/api/v1/mcp`} />
        </SettingsCard>
      )}
    </div>
  )
}

/** Réglages › Mises à jour : version, vérification, mise à jour (immédiate ou automatique). */
export function AdminUpdatesPage() {
  const qc = useQueryClient()
  const data = useSettings().data
  const [confirm, setConfirm] = useState(false)
  const check = useMutation({
    mutationFn: () => post<{ latest_version: string; update_available: boolean }>("/admin/update-check"),
    onSuccess: (r) => {
      toast.success(r.update_available ? `Version ${r.latest_version} disponible` : "Nodra est à jour")
      void qc.invalidateQueries({ queryKey: settingsKey })
    },
    onError: (e) => toast.error(e.message),
  })
  const update = useMutation({
    mutationFn: () => post("/admin/update", {}),
    onSuccess: () => {
      setConfirm(false)
      toast.success("Mise à jour lancée", { description: "L'interface sera indisponible quelques instants." })
    },
    onError: (e) => toast.error(e.message),
  })
  if (!data) return null
  return (
    <>
      <PageHeader title="Mises à jour" />
      <div className="space-y-6">
        <SettingsCard
          title="Version"
          actions={
            <>
              <Button size="sm" variant="outline" loading={check.isPending} onClick={() => check.mutate()}>
                <RefreshCwIcon /> Vérifier
              </Button>
              {data.update_available && (
                <Button size="sm" onClick={() => setConfirm(true)}>
                  Mettre à jour vers {data.settings.latest_version}
                </Button>
              )}
            </>
          }
        >
          <p className="text-sm">
            Version installée : <strong className="font-mono">{data.version}</strong>
            {data.settings.latest_version && (
              <>
                {" "}
                · dernière : <strong className="font-mono">{data.settings.latest_version}</strong>
              </>
            )}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">Vérifiée {relativeTime(data.settings.update_checked_at)}</p>
        </SettingsCard>
        <AutoUpdate key={JSON.stringify(data.settings)} s={data.settings} />
      </div>
      <ConfirmDialog open={confirm} onOpenChange={setConfirm} title="Mettre à jour Nodra ?" description="update.sh est lancé sur le serveur de l'instance (installation par install.sh requise)." destructive={false} actionLabel="Mettre à jour" loading={update.isPending} onConfirm={() => update.mutate()} />
    </>
  )
}

function AutoUpdate({ s }: { s: Settings }) {
  const { draft: d, set, changed, dirty } = useDraft({ auto_update_enabled: s.auto_update_enabled, auto_update_frequency: s.auto_update_frequency, update_check_frequency: s.update_check_frequency })
  const save = usePatchSettings()
  return (
    <SettingsCard title="Automatique" actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(changed)} />}>
      <SwitchRow label="Mise à jour automatique" hint="Au plus une fois par version et par jour, à l'échéance cron." checked={d.auto_update_enabled} onChange={(v) => set("auto_update_enabled", v)} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Créneau de mise à jour (cron)">
          <Input value={d.auto_update_frequency} onChange={(e) => set("auto_update_frequency", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Vérification des versions (cron)">
          <Input value={d.update_check_frequency} onChange={(e) => set("update_check_frequency", e.target.value)} className="font-mono" />
        </Field>
      </div>
    </SettingsCard>
  )
}

type Provider = { provider: string; name: string; enabled: boolean; client_id: string; client_secret: string; base_url: string; tenant: string; redirect_uri: string }

/** Réglages › Authentification : fournisseurs de connexion OAuth/OIDC. */
export function AdminOAuthPage() {
  const providers = useQuery({ queryKey: ["admin", "oauth"], queryFn: () => get<Provider[]>("/admin/oauth") }).data ?? []
  return (
    <>
      <PageHeader title="Fournisseurs de connexion" description="Connexion avec GitHub, GitLab, Google, Microsoft, Discord ou un fournisseur OIDC." />
      <div className="space-y-4">
        {providers.map((p) => (
          <ProviderCard key={p.provider + p.enabled + p.client_id} p={p} />
        ))}
      </div>
    </>
  )
}

function ProviderCard({ p }: { p: Provider }) {
  const qc = useQueryClient()
  const [d, setD] = useState({ enabled: p.enabled, client_id: p.client_id, client_secret: p.client_secret, base_url: p.base_url, tenant: p.tenant })
  const save = useMutation({
    mutationFn: () => api(`/admin/oauth/${p.provider}`, { method: "PUT", json: d }),
    onSuccess: () => (toast.success(`${p.name} enregistré`), void qc.invalidateQueries({ queryKey: ["admin", "oauth"] })),
    onError: (e) => toast.error(e.message),
  })
  const needsBase = ["github", "gitlab", "oidc"].includes(p.provider)
  return (
    <SettingsCard
      title={p.name}
      actions={
        <>
          <Pill label={p.enabled ? "Activé" : "Désactivé"} dot={p.enabled ? "bg-success" : "bg-muted-foreground"} />
          <Button size="sm" variant="outline" loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
        </>
      }
    >
      <SwitchRow label="Activé" checked={d.enabled} onChange={(v) => setD({ ...d, enabled: v })} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Client ID">
          <Input value={d.client_id} onChange={(e) => setD({ ...d, client_id: e.target.value })} className="font-mono" />
        </Field>
        <Field label="Secret client">
          <Input type="password" value={d.client_secret} onChange={(e) => setD({ ...d, client_secret: e.target.value })} className="font-mono" />
        </Field>
        {needsBase && (
          <Field label={p.provider === "oidc" ? "Émetteur (obligatoire)" : "Adresse (auto-hébergé)"}>
            <Input value={d.base_url} onChange={(e) => setD({ ...d, base_url: e.target.value })} className="font-mono" />
          </Field>
        )}
        {p.provider === "microsoft" && (
          <Field label="Tenant">
            <Input value={d.tenant} onChange={(e) => setD({ ...d, tenant: e.target.value })} className="font-mono" placeholder="common" />
          </Field>
        )}
        <Field label="URL de redirection (à déclarer chez le fournisseur)" className="sm:col-span-2">
          <CopyInput value={p.redirect_uri} />
        </Field>
      </div>
    </SettingsCard>
  )
}

type Mail = { configured: boolean; config?: { provider: "smtp" | "resend"; from_address: string; from_name: string; host?: string; port?: number; encryption?: string; username?: string; password?: string; resend_api_key?: string } }

/** Réglages › E-mail : SMTP ou Resend, test d'envoi. */
export function AdminEmailPage() {
  const mail = useQuery({ queryKey: ["admin", "mail"], queryFn: () => get<Mail>("/admin/mail") }).data
  return (
    <>
      <PageHeader title="E-mail de l'instance" description="Réinitialisation de mot de passe, invitations, vérification d'adresse et canaux de notification e-mail." />
      {mail && <MailForm key={JSON.stringify(mail)} mail={mail} />}
    </>
  )
}

function MailForm({ mail }: { mail: Mail }) {
  const qc = useQueryClient()
  const c = mail.config
  const [d, setD] = useState({
    provider: c?.provider ?? "smtp",
    from_address: c?.from_address ?? "",
    from_name: c?.from_name ?? "",
    host: c?.host ?? "",
    port: String(c?.port ?? 587),
    encryption: c?.encryption ?? "starttls",
    username: c?.username ?? "",
    password: c?.password ?? "",
    resend_api_key: c?.resend_api_key ?? "",
  })
  const [to, setTo] = useState("")
  const [removing, setRemoving] = useState(false)
  const refresh = () => void qc.invalidateQueries({ queryKey: ["admin", "mail"] })
  const save = useMutation({
    mutationFn: () =>
      api("/admin/mail", {
        method: "PUT",
        json:
          d.provider === "resend"
            ? { provider: "resend", resend_api_key: d.resend_api_key, from_address: d.from_address, from_name: d.from_name }
            : { provider: "smtp", from_address: d.from_address, from_name: d.from_name, host: d.host, port: Number(d.port), encryption: d.encryption, username: d.username, password: d.password },
      }),
    onSuccess: () => (toast.success("E-mail enregistré"), refresh()),
    onError: (e) => toast.error(e.message),
  })
  const test = useMutation({ mutationFn: () => post("/admin/mail/test", to ? { to } : {}), onSuccess: () => toast.success("E-mail de test envoyé"), onError: (e) => toast.error(e.message) })
  const remove = useMutation({ mutationFn: () => api("/admin/mail", { method: "DELETE" }), onSuccess: () => (setRemoving(false), refresh()), onError: (e) => toast.error(e.message) })
  const f = (k: keyof typeof d, label: string, secret?: boolean) => (
    <Field label={label}>
      <Input value={d[k]} onChange={(e) => setD({ ...d, [k]: e.target.value })} type={secret ? "password" : "text"} className="font-mono" />
    </Field>
  )
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Configuration"
        actions={
          <>
            <Pill label={mail.configured ? "Configuré" : "Non configuré"} dot={mail.configured ? "bg-success" : "bg-warning"} />
            {mail.configured && (
              <Button size="sm" variant="ghost" onClick={() => setRemoving(true)}>
                <Trash2Icon /> Retirer
              </Button>
            )}
            <Button size="sm" variant="outline" loading={save.isPending} onClick={() => save.mutate()}>
              Enregistrer
            </Button>
          </>
        }
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Service">
            <Select value={d.provider} onValueChange={(v) => setD({ ...d, provider: v as "smtp" | "resend" })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="smtp">SMTP</SelectItem>
                <SelectItem value="resend">Resend</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <div />
          {f("from_address", "Adresse d'expédition")}
          {f("from_name", "Nom d'expédition")}
          {d.provider === "smtp" ? (
            <>
              {f("host", "Serveur SMTP")}
              <div className="grid grid-cols-2 gap-4">
                {f("port", "Port")}
                <Field label="Chiffrement">
                  <Select value={d.encryption} onValueChange={(v) => setD({ ...d, encryption: v })}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="tls">TLS</SelectItem>
                      <SelectItem value="starttls">STARTTLS</SelectItem>
                      <SelectItem value="none">Aucun</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              </div>
              {f("username", "Utilisateur")}
              {f("password", "Mot de passe", true)}
            </>
          ) : (
            f("resend_api_key", "Clé API Resend", true)
          )}
        </div>
      </SettingsCard>
      {mail.configured && (
        <SettingsCard title="Test">
          <div className="flex max-w-lg gap-2">
            <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="Destinataire (vous par défaut)" />
            <Button size="sm" variant="outline" loading={test.isPending} onClick={() => test.mutate()}>
              <SendIcon /> Envoyer
            </Button>
          </div>
        </SettingsCard>
      )}
      <ConfirmDialog open={removing} onOpenChange={setRemoving} title="Retirer la configuration e-mail ?" actionLabel="Retirer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </div>
  )
}

type AdminUser = { id: string; email: string; name: string; is_instance_admin: boolean; two_factor_enabled: boolean; teams: number; last_seen_at: string | null; disabled_at: string | null; created_at: string }

/** Réglages › Utilisateurs : administrateurs, désactivation, déconnexion forcée, suppression. */
export function AdminUsersPage() {
  const qc = useQueryClient()
  const me = useQuery(meQuery).data?.user
  const [search, setSearch] = useState("")
  const users = useQuery({ queryKey: ["admin", "users", search], queryFn: () => get<AdminUser[]>(`/admin/users?search=${encodeURIComponent(search)}`) }).data ?? []
  const [deleting, setDeleting] = useState<AdminUser | null>(null)
  const refresh = () => void qc.invalidateQueries({ queryKey: ["admin", "users"] })
  const patch = useMutation({ mutationFn: ({ id, json }: { id: string; json: object }) => api(`/admin/users/${id}`, { method: "PATCH", json }), onSuccess: refresh, onError: (e) => toast.error(e.message) })
  const logout = useMutation({ mutationFn: (id: string) => post(`/admin/users/${id}/logout`), onSuccess: () => toast.success("Sessions révoquées"), onError: (e) => toast.error(e.message) })
  const remove = useMutation({ mutationFn: (u: AdminUser) => api(`/admin/users/${u.id}`, { method: "DELETE" }), onSuccess: () => (setDeleting(null), refresh()), onError: (e) => toast.error(e.message) })
  return (
    <>
      <PageHeader title="Utilisateurs" />
      <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Rechercher (nom ou e-mail)" className="mb-4 max-w-sm" />
      <SettingsCard title="Comptes" bodyClassName="p-0">
        {users.map((u) => (
          <div key={u.id} className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {u.name || u.email} {u.id === me?.id && <span className="text-xs text-muted-foreground">(vous)</span>}
              </p>
              <p className="truncate text-xs text-muted-foreground" title={dateTime(u.created_at)}>
                {u.email} · {u.teams} équipe(s) · vu {relativeTime(u.last_seen_at)}
              </p>
            </div>
            {u.is_instance_admin && <Pill label="Admin" dot="bg-primary" />}
            {u.two_factor_enabled && <Pill label="2FA" dot="bg-success" />}
            {u.disabled_at && <Pill label="Désactivé" dot="bg-destructive" />}
            {u.id !== me?.id && (
              <>
                <Button size="sm" variant="ghost" onClick={() => patch.mutate({ id: u.id, json: { is_instance_admin: !u.is_instance_admin } })}>
                  <ShieldIcon /> {u.is_instance_admin ? "Retirer admin" : "Rendre admin"}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => patch.mutate({ id: u.id, json: { disabled: !u.disabled_at } })}>
                  {u.disabled_at ? "Réactiver" : "Désactiver"}
                </Button>
                <Button size="sm" variant="ghost" aria-label="Déconnecter" onClick={() => logout.mutate(u.id)}>
                  <LogOutIcon />
                </Button>
                <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(u)}>
                  <Trash2Icon />
                </Button>
              </>
            )}
          </div>
        ))}
      </SettingsCard>
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={`Supprimer ${deleting?.email} ?`} description="Le compte et son équipe personnelle sont supprimés." confirmText={deleting?.email} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => deleting && remove.mutate(deleting)} />
    </>
  )
}

/** Réglages › Audit de l'instance : événements sans équipe (connexions, réglages) ou de toute l'instance. */
export function AdminAuditPage() {
  const [scope, setScope] = useState("instance")
  return (
    <>
      <div className="mb-2 flex justify-end">
        <Select value={scope} onValueChange={setScope}>
          <SelectTrigger className="h-8 w-56">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="instance">Instance (connexions, réglages)</SelectItem>
            <SelectItem value="all">Toute l'instance</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <AuditLog key={scope} base={`/admin/audit`} scopeParam={scope} title="Audit de l'instance" />
    </>
  )
}
