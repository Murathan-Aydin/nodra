import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { SendIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { EmptyPanel, PageHeader, SettingsCard } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { api, get, post } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { cn } from "@/lib/utils"

type ChannelType = "email" | "discord" | "slack" | "telegram" | "pushover" | "webhook"
type Channel = { id: string; type: ChannelType; name: string; config: Record<string, unknown>; events: string[]; enabled: boolean }
type Catalog = { events: { key: string; label: string; default: boolean }[]; channel_types: string[] }

const TYPES: Record<ChannelType, { label: string; fields: [string, string, boolean?][] }> = {
  email: { label: "E-mail", fields: [["recipients", "Destinataires (séparés par des virgules)"]] },
  discord: { label: "Discord", fields: [["webhook_url", "URL du webhook", true]] },
  slack: { label: "Slack", fields: [["webhook_url", "URL du webhook", true]] },
  telegram: { label: "Telegram", fields: [["bot_token", "Jeton du bot", true], ["chat_id", "Identifiant du chat"]] },
  pushover: { label: "Pushover", fields: [["user_key", "Clé utilisateur", true], ["api_token", "Jeton d'application", true]] },
  webhook: { label: "Webhook JSON", fields: [["webhook_url", "URL", true]] },
}

const catalogQ = { queryKey: ["notifications", "catalog"], queryFn: () => get<Catalog>("/notifications/catalog"), staleTime: Infinity }
const channelsQ = (team: string) => ({ queryKey: ["team", team, "notification-channels"], queryFn: () => get<Channel[]>(`/teams/${team}/notification-channels`) })

/** Page « Notifications › Canaux » (notifications du fork : e-mail, Discord, Telegram, Slack, Pushover, webhook). */
export function ChannelsPage({ team }: { team: string }) {
  const channels = useQuery(channelsQ(team)).data ?? []
  const [adding, setAdding] = useState<ChannelType | "">("")
  return (
    <>
      <PageHeader
        title="Notifications"
        description="Un canal de l'équipe reçoit aussi les événements de ses sous-équipes."
        actions={
          <Select value={adding} onValueChange={(v) => setAdding(v as ChannelType)}>
            <SelectTrigger className="h-8 w-48">
              <SelectValue placeholder="Ajouter un canal" />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(TYPES).map(([k, t]) => (
                <SelectItem key={k} value={k}>
                  {t.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />
      <div className="space-y-4">
        {adding && <ChannelCard key={adding} team={team} channel={{ id: "", type: adding, name: TYPES[adding].label, config: {}, events: [], enabled: true }} onDone={() => setAdding("")} />}
        {channels.map((c) => (
          <ChannelCard key={c.id + JSON.stringify(c)} team={team} channel={c} />
        ))}
        {!adding && channels.length === 0 && <EmptyPanel title="Aucun canal" text="Choisissez un type en haut à droite pour en ajouter un." />}
      </div>
    </>
  )
}

function ChannelCard({ team, channel, onDone }: { team: string; channel: Channel; onDone?: () => void }) {
  const qc = useQueryClient()
  const catalog = useQuery(catalogQ).data
  const isNew = !channel.id
  const t = TYPES[channel.type]
  const [name, setName] = useState(channel.name)
  // Champs texte du formulaire ; les tableaux (destinataires) sont joints, les objets (sujets Telegram) gardés à part.
  const [config, setConfig] = useState<Record<string, string>>(() =>
    Object.fromEntries(Object.entries(channel.config ?? {}).filter(([, v]) => typeof v !== "object" || Array.isArray(v)).map(([k, v]) => [k, Array.isArray(v) ? v.join(", ") : String(v ?? "")])),
  )
  const [events, setEvents] = useState(() => new Set(isNew ? (catalog?.events.filter((e) => e.default).map((e) => e.key) ?? []) : channel.events))
  const [enabled, setEnabled] = useState(channel.enabled)
  const [deleting, setDeleting] = useState(false)
  const refresh = () => void qc.invalidateQueries({ queryKey: ["team", team, "notification-channels"] })
  const body = () => {
    const c: Record<string, unknown> = Object.fromEntries(t.fields.map(([k]) => [k, config[k] ?? ""]))
    if (channel.type === "email") c.recipients = (config.recipients ?? "").split(/[\s,]+/).filter(Boolean)
    if (channel.type === "discord") c.ping = config.ping === "true"
    if (channel.type === "telegram" && channel.config?.threads) c.threads = channel.config.threads
    return { name, enabled, events: [...events], config: c }
  }
  const save = useMutation({
    mutationFn: () => (isNew ? post(`/teams/${team}/notification-channels`, { type: channel.type, ...body() }) : api(`/teams/${team}/notification-channels/${channel.id}`, { method: "PATCH", json: body() })),
    onSuccess: () => (toast.success("Canal enregistré"), refresh(), onDone?.()),
    onError: (e) => toast.error(e.message),
  })
  const test = useMutation({ mutationFn: () => post(`/teams/${team}/notification-channels/${channel.id}/test`), onSuccess: () => toast.success("Message de test envoyé"), onError: (e) => toast.error(e.message) })
  const remove = useMutation({ mutationFn: () => api(`/teams/${team}/notification-channels/${channel.id}`, { method: "DELETE" }), onSuccess: refresh, onError: (e) => toast.error(e.message) })
  return (
    <SettingsCard
      title={isNew ? `Nouveau canal ${t.label}` : channel.name}
      description={t.label}
      actions={
        <>
          <Switch checked={enabled} onCheckedChange={setEnabled} aria-label="Activé" />
          {!isNew && (
            <Button size="sm" variant="ghost" loading={test.isPending} onClick={() => test.mutate()}>
              <SendIcon /> Tester
            </Button>
          )}
          {isNew ? (
            <Button size="sm" variant="ghost" onClick={onDone}>
              Annuler
            </Button>
          ) : (
            <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
              <Trash2Icon />
            </Button>
          )}
          <Button size="sm" variant="outline" loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
        </>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Nom">
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        {t.fields.map(([k, label, secret]) => (
          <Field key={k} label={label}>
            <Input value={config[k] ?? ""} onChange={(e) => setConfig({ ...config, [k]: e.target.value })} type={secret ? "password" : "text"} className="font-mono" />
          </Field>
        ))}
        {channel.type === "discord" && (
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={config.ping === "true"} onCheckedChange={(c) => setConfig({ ...config, ping: c ? "true" : "false" })} /> Mentionner @here sur les erreurs
          </label>
        )}
      </div>
      <p className="mt-5 mb-2 text-xs font-semibold text-muted-foreground">Événements</p>
      <div className="grid gap-1.5 sm:grid-cols-2 lg:grid-cols-3">
        {catalog?.events.map((e) => (
          <label key={e.key} className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={events.has(e.key)}
              onCheckedChange={(c) =>
                setEvents((s) => {
                  const n = new Set(s)
                  if (c) n.add(e.key)
                  else n.delete(e.key)
                  return n
                })
              }
            />
            {e.label}
          </label>
        ))}
      </div>
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} title={`Supprimer « ${channel.name} » ?`} actionLabel="Supprimer" loading={remove.isPending} onConfirm={() => remove.mutate()} />
    </SettingsCard>
  )
}

/** Page « Notifications › Abonnements » : vue d'ensemble événements × canaux. */
export function SubscriptionsPage({ team }: { team: string }) {
  const catalog = useQuery(catalogQ).data
  const channels = useQuery(channelsQ(team)).data ?? []
  return (
    <>
      <PageHeader title="Abonnements" description="Quels canaux reçoivent quels événements (à modifier dans chaque canal)." />
      <SettingsCard title="Événements" bodyClassName="overflow-x-auto p-0">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-[11px] text-muted-foreground">
              <th className="px-4 py-2.5 font-medium">Événement</th>
              {channels.map((c) => (
                <th key={c.id} className="px-3 py-2.5 text-center font-medium">
                  {c.name}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {catalog?.events.map((e) => (
              <tr key={e.key} className="border-b border-border last:border-b-0">
                <td className="px-4 py-2">{e.label}</td>
                {channels.map((c) => (
                  <td key={c.id} className="px-3 py-2 text-center">
                    <span className={cn("inline-block size-2 rounded-full", c.events.includes(e.key) && c.enabled ? "bg-success" : "bg-muted")} />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </SettingsCard>
    </>
  )
}

type Delivery = { id: string; channel_id: string; event: string; title: string; status: "sent" | "failed"; error: string; created_at: string }

/** Page « Notifications › Journal des envois » (30 jours). */
export function DeliveriesPage({ team }: { team: string }) {
  const list = useQuery({ queryKey: ["team", team, "deliveries"], queryFn: () => get<Delivery[]>(`/teams/${team}/notification-deliveries?limit=100`), refetchInterval: 30_000 }).data ?? []
  const channels = useQuery(channelsQ(team)).data ?? []
  return (
    <>
      <PageHeader title="Journal des envois" description="100 derniers envois (conservés 30 jours)." />
      <SettingsCard title="Envois" bodyClassName="p-0">
        {list.map((d) => (
          <div key={d.id} className="flex items-center gap-3 border-b border-border px-4 py-3 last:border-b-0">
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm">{d.title}</p>
              <p className="truncate text-xs text-muted-foreground" title={dateTime(d.created_at)}>
                {channels.find((c) => c.id === d.channel_id)?.name ?? "Canal supprimé"} · {d.event} · {relativeTime(d.created_at)}
                {d.error && ` · ${d.error}`}
              </p>
            </div>
            <Pill label={d.status === "sent" ? "Envoyé" : "Échec"} dot={d.status === "sent" ? "bg-success" : "bg-destructive"} />
          </div>
        ))}
        {list.length === 0 && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucun envoi.</p>}
      </SettingsCard>
    </>
  )
}
