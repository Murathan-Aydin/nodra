import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import QRCode from "qrcode"
import { LinkIcon, LogOutIcon, ShieldCheckIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { PageHeader, SettingsCard } from "@/components/page"
import { PasswordInput } from "@/components/password-input"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api, get, post } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { instanceQuery, meQuery } from "@/lib/queries"
import { AppearanceSection } from "./appearance"

/** Page « Mon compte » (Profile du fork) : profil, mot de passe, 2FA, sessions, comptes liés, apparence. */
export function AccountPage() {
  return (
    <>
      <PageHeader title="Mon compte" />
      <div className="space-y-6">
        <Profile />
        <Password />
        <TwoFactor />
        <Sessions />
        <Identities />
        <AppearanceSection />
      </div>
    </>
  )
}

function Profile() {
  const qc = useQueryClient()
  const me = useQuery(meQuery).data?.user
  const [name, setName] = useState<string | null>(null)
  const value = name ?? me?.name ?? ""
  const save = useMutation({
    mutationFn: () => api("/me", { method: "PATCH", json: { name: value } }),
    onSuccess: () => (toast.success("Profil enregistré"), setName(null), void qc.invalidateQueries({ queryKey: ["me"] })),
    onError: (e) => toast.error(e.message),
  })
  const verify = useMutation({ mutationFn: () => post("/me/verify-email"), onSuccess: () => toast.success("Lien de vérification envoyé"), onError: (e) => toast.error(e.message) })
  if (!me) return null
  return (
    <SettingsCard
      title="Profil"
      actions={
        <Button size="sm" variant="outline" disabled={name === null || name === me.name} loading={save.isPending} onClick={() => save.mutate()}>
          Enregistrer
        </Button>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Nom">
          <Input value={value} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="E-mail">
          <div className="flex items-center gap-2">
            <Input value={me.email} readOnly />
            {me.email_verified_at ? (
              <Pill label="Vérifiée" dot="bg-success" />
            ) : (
              <Button size="sm" variant="ghost" loading={verify.isPending} onClick={() => verify.mutate()}>
                Vérifier
              </Button>
            )}
          </div>
        </Field>
      </div>
    </SettingsCard>
  )
}

function Password() {
  const [d, setD] = useState({ current_password: "", new_password: "" })
  const save = useMutation({
    mutationFn: () => post("/me/password", d),
    onSuccess: () => (toast.success("Mot de passe changé", { description: "Vos autres sessions ont été fermées." }), setD({ current_password: "", new_password: "" })),
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title="Mot de passe"
      actions={
        <Button size="sm" variant="outline" disabled={!d.current_password || d.new_password.length < 8} loading={save.isPending} onClick={() => save.mutate()}>
          Changer
        </Button>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Mot de passe actuel">
          <PasswordInput value={d.current_password} onChange={(e) => setD({ ...d, current_password: e.target.value })} autoComplete="current-password" />
        </Field>
        <Field label="Nouveau mot de passe" hint="8 caractères minimum.">
          <PasswordInput value={d.new_password} onChange={(e) => setD({ ...d, new_password: e.target.value })} autoComplete="new-password" />
        </Field>
      </div>
    </SettingsCard>
  )
}

function TwoFactor() {
  const qc = useQueryClient()
  const enabled = useQuery(meQuery).data?.two_factor_enabled ?? false
  const [setup, setSetup] = useState<{ secret: string; otpauth_url: string } | null>(null)
  const [qr, setQr] = useState("")
  const [code, setCode] = useState("")
  const [codes, setCodes] = useState<string[]>([])
  const [disabling, setDisabling] = useState(false)
  const [password, setPassword] = useState("")
  useEffect(() => {
    if (setup) void QRCode.toDataURL(setup.otpauth_url, { margin: 1, width: 180 }).then(setQr)
  }, [setup])
  const start = useMutation({ mutationFn: () => post<{ secret: string; otpauth_url: string }>("/me/2fa/setup"), onSuccess: setSetup, onError: (e) => toast.error(e.message) })
  const enable = useMutation({
    mutationFn: () => post<{ recovery_codes: string[] }>("/me/2fa/enable", { code }),
    onSuccess: (r) => {
      setCodes(r.recovery_codes)
      setSetup(null)
      void qc.invalidateQueries({ queryKey: ["me"] })
    },
    onError: (e) => toast.error(e.message),
  })
  const disable = useMutation({
    mutationFn: () => post("/me/2fa/disable", { password }),
    onSuccess: () => (setDisabling(false), setPassword(""), toast.success("Double authentification désactivée"), void qc.invalidateQueries({ queryKey: ["me"] })),
    onError: (e) => toast.error(e.message),
  })
  return (
    <SettingsCard
      title="Double authentification"
      description="Code à usage unique d'une application (Authy, 1Password, Google Authenticator…) demandé à la connexion."
      actions={
        enabled ? (
          <>
            <Pill label="Activée" dot="bg-success" />
            <Button size="sm" variant="ghost" onClick={() => setDisabling(true)}>
              Désactiver
            </Button>
          </>
        ) : (
          !setup && (
            <Button size="sm" variant="outline" loading={start.isPending} onClick={() => start.mutate()}>
              <ShieldCheckIcon /> Activer
            </Button>
          )
        )
      }
    >
      {codes.length > 0 ? (
        <div className="space-y-2">
          <p className="text-sm">Codes de secours (à conserver en lieu sûr, affichés une seule fois) :</p>
          <CopyInput value={codes.join(" ")} />
        </div>
      ) : setup ? (
        <div className="flex flex-wrap items-start gap-6">
          {qr && <img src={qr} alt="QR code à scanner" className="rounded-lg bg-white p-1" />}
          <div className="min-w-64 flex-1 space-y-4">
            <Field label="Ou saisissez la clé">
              <CopyInput value={setup.secret} />
            </Field>
            <Field label="Code à 6 chiffres">
              <div className="flex gap-2">
                <Input value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))} className="max-w-32 font-mono" inputMode="numeric" />
                <Button size="sm" disabled={code.length !== 6} loading={enable.isPending} onClick={() => enable.mutate()}>
                  Confirmer
                </Button>
              </div>
            </Field>
          </div>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{enabled ? "Votre compte est protégé." : "Non activée."}</p>
      )}
      <ConfirmDialog open={disabling} onOpenChange={setDisabling} title="Désactiver la double authentification ?" actionLabel="Désactiver" loading={disable.isPending} onConfirm={() => disable.mutate()}>
        <Field label="Mot de passe">
          <PasswordInput value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
      </ConfirmDialog>
    </SettingsCard>
  )
}

type Session = { id: string; ip: string; user_agent: string; created_at: string; last_seen_at: string; expires_at: string; current: boolean }

function Sessions() {
  const qc = useQueryClient()
  const sessions = useQuery({ queryKey: ["me", "sessions"], queryFn: () => get<Session[]>("/me/sessions") }).data ?? []
  const revoke = useMutation({ mutationFn: (id: string) => api(`/me/sessions/${id}`, { method: "DELETE" }), onSuccess: () => void qc.invalidateQueries({ queryKey: ["me", "sessions"] }), onError: (e) => toast.error(e.message) })
  return (
    <SettingsCard title="Sessions" bodyClassName="p-0">
      {sessions.map((s) => (
        <div key={s.id} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm">{browser(s.user_agent)}</p>
            <p className="truncate text-xs text-muted-foreground" title={dateTime(s.created_at)}>
              {s.ip} · active {relativeTime(s.last_seen_at)} · expire {relativeTime(s.expires_at)}
            </p>
          </div>
          {s.current ? (
            <Pill label="Cette session" dot="bg-success" />
          ) : (
            <Button size="sm" variant="ghost" onClick={() => revoke.mutate(s.id)}>
              <LogOutIcon /> Fermer
            </Button>
          )}
        </div>
      ))}
    </SettingsCard>
  )
}

/** Navigateur et système lisibles depuis un user-agent. */
function browser(ua: string): string {
  const b = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Navigateur"
  const os = /Mac OS X/.test(ua) ? "macOS" : /Windows/.test(ua) ? "Windows" : /Android/.test(ua) ? "Android" : /iPhone|iPad/.test(ua) ? "iOS" : /Linux/.test(ua) ? "Linux" : ""
  return ua ? `${b}${os ? ` sur ${os}` : ""}` : "Client inconnu"
}

type Identity = { provider: string; subject: string; email: string; created_at: string }

function Identities() {
  const qc = useQueryClient()
  const providers = useQuery(instanceQuery).data?.oauth_providers ?? []
  const ids = useQuery({ queryKey: ["me", "identities"], queryFn: () => get<Identity[]>("/me/identities") }).data ?? []
  const [unlinking, setUnlinking] = useState<string | null>(null)
  const unlink = useMutation({
    mutationFn: (p: string) => api(`/me/identities/${p}`, { method: "DELETE" }),
    onSuccess: () => (setUnlinking(null), void qc.invalidateQueries({ queryKey: ["me", "identities"] })),
    onError: (e) => toast.error(e.message),
  })
  if (providers.length === 0 && ids.length === 0) return null
  return (
    <SettingsCard title="Comptes liés" description="Se connecter avec un compte externe." bodyClassName="p-0">
      {providers.map((p) => {
        const id = ids.find((i) => i.provider === p.provider)
        return (
          <div key={p.provider} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">{p.name}</p>
              <p className="text-xs text-muted-foreground">{id ? `${id.email} · lié ${relativeTime(id.created_at)}` : "Non lié"}</p>
            </div>
            {id ? (
              <Button size="sm" variant="ghost" onClick={() => setUnlinking(p.provider)}>
                <Trash2Icon /> Délier
              </Button>
            ) : (
              <Button size="sm" variant="outline" asChild>
                <a href={`/api/v1/auth/oauth/${p.provider}/start?link=1&redirect=${encodeURIComponent(location.pathname)}`}>
                  <LinkIcon /> Lier
                </a>
              </Button>
            )}
          </div>
        )
      })}
      <ConfirmDialog open={!!unlinking} onOpenChange={(o) => !o && setUnlinking(null)} title="Délier ce compte ?" description="Refusé si c'est votre seul moyen de connexion." actionLabel="Délier" loading={unlink.isPending} onConfirm={() => unlinking && unlink.mutate(unlinking)} />
    </SettingsCard>
  )
}
