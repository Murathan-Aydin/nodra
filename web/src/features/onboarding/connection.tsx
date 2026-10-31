import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { CheckCircle2Icon, CopyIcon, KeyRoundIcon, LoaderCircleIcon, RefreshCwIcon, SparklesIcon, XCircleIcon } from "lucide-react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { ApiError, get, post, teamPath } from "@/lib/api"
import type { PrivateKey, Server, ServerTask } from "@/lib/types"
import { cn } from "@/lib/utils"
import { ChoiceCard, TaskLog, useServerTask } from "./ui"

const errorText = (e: unknown, fallback: string) => (e instanceof ApiError ? e.message : fallback)

/** État d'un serveur pendant sa validation (connexion SSH, OS, Docker). */
export function ServerStatus({ server, task }: { server?: Server; task?: ServerTask }) {
  const running = !task || task.status === "queued" || task.status === "running"
  const ok = server?.is_usable && !running
  return (
    <div
      className={cn(
        "flex items-start gap-3 rounded-xl border p-4 text-sm",
        ok ? "border-success/30 bg-success/10" : running ? "border-border bg-muted" : "border-destructive/30 bg-destructive/10",
      )}
    >
      {ok ? (
        <CheckCircle2Icon className="size-5 shrink-0 text-success" />
      ) : running ? (
        <LoaderCircleIcon className="size-5 shrink-0 animate-spin text-muted-foreground" />
      ) : (
        <XCircleIcon className="size-5 shrink-0 text-destructive" />
      )}
      <div className="min-w-0">
        <p className="font-medium">{ok ? "Serveur prêt" : running ? "Validation en cours…" : "Validation échouée"}</p>
        <p className="text-muted-foreground">
          {ok
            ? `${server?.os_id ?? ""} ${server?.os_version ?? ""} · Docker ${server?.docker_version ?? ""}`
            : running
              ? "Connexion SSH, système, Docker et réseau de Nodra."
              : server?.validation_error || task?.error || "Voir le détail ci-dessous."}
        </p>
      </div>
    </div>
  )
}

/** « Cette machine » : serveur créé automatiquement à la première inscription (install.sh). */
export function LocalhostConnection({ team, onReady }: { team: string; onReady: (server: Server) => void }) {
  const qc = useQueryClient()
  const [task, setTask] = useState<string>()
  const servers = useQuery({
    queryKey: ["team", team, "servers"],
    queryFn: () => get<Server[]>(teamPath(team, "/servers")),
    refetchInterval: (q) => (q.state.data?.find((s) => s.is_localhost)?.is_usable ? false : 2000),
  })
  const server = servers.data?.find((s) => s.is_localhost)
  // Dernière validation lancée par l'inscription, si aucune n'a été relancée ici.
  const latest = useQuery({
    queryKey: ["team", team, "server", server?.id, "tasks"],
    queryFn: () => get<ServerTask[]>(teamPath(team, `/servers/${server!.id}/tasks`)),
    enabled: !!server && !task,
  })
  const taskId = task ?? latest.data?.find((t) => t.kind === "validate")?.id
  const current = useServerTask(team, server?.id, taskId)
  const revalidate = useMutation({
    mutationFn: () => post<ServerTask>(teamPath(team, `/servers/${server!.id}/validate`)),
    onSuccess: (t) => {
      setTask(t.id)
      void qc.invalidateQueries({ queryKey: ["team", team, "servers"] })
    },
    onError: (e) => toast.error(errorText(e, "Validation impossible")),
  })
  if (servers.isPending) return <p className="text-sm text-muted-foreground">Recherche du serveur hôte…</p>
  if (!server)
    return (
      <p className="rounded-xl border border-warning/40 bg-warning/10 p-4 text-sm">
        Le serveur hôte n'a pas été trouvé. Il est créé à la première inscription quand Nodra est installé avec <code className="font-mono">install.sh</code> ;
        choisissez « Adresse IP ou domaine » pour connecter un serveur à la main.
      </p>
    )
  const finished = current.data && current.data.status !== "queued" && current.data.status !== "running"
  return (
    <div className="space-y-4">
      <ServerStatus server={server} task={current.data} />
      <TaskLog task={current.data} />
      <div className="flex justify-end gap-2">
        {finished && !server.is_usable && (
          <Button variant="outline" loading={revalidate.isPending} onClick={() => revalidate.mutate()}>
            <RefreshCwIcon /> Relancer la validation
          </Button>
        )}
        <Button disabled={!server.is_usable} onClick={() => onReady(server)}>
          Continuer
        </Button>
      </div>
    </div>
  )
}

type KeyMode = "generate" | "paste"

/** Serveur distant : clé SSH (générée ou importée), puis connexion et validation. */
export function RemoteConnection({ team, onReady }: { team: string; onReady: (server: Server) => void }) {
  const qc = useQueryClient()
  const [mode, setMode] = useState<KeyMode>("generate")
  const [pasted, setPasted] = useState("")
  const [key, setKey] = useState<PrivateKey | null>(null)
  const [f, setF] = useState({ name: "", ip: "", port: "22", user: "root" })
  const [created, setCreated] = useState<{ server: Server; task: string } | null>(null)

  const makeKey = useMutation({
    mutationFn: () => post<PrivateKey>(teamPath(team, "/private-keys"), { name: `cle-${f.name.trim() || "serveur"}`, private_key: mode === "paste" ? pasted : "" }),
    onSuccess: setKey,
    onError: (e) => toast.error(errorText(e, "Clé refusée")),
  })
  const connect = useMutation({
    mutationFn: () =>
      post<{ server: Server; task: ServerTask }>(teamPath(team, "/servers"), {
        name: f.name.trim() || f.ip.trim(),
        ip: f.ip.trim(),
        port: Number(f.port) || 22,
        user: f.user.trim() || "root",
        private_key_id: key!.id,
      }),
    onSuccess: (r) => setCreated({ server: r.server, task: r.task.id }),
    onError: (e) => toast.error(errorText(e, "Ajout impossible")),
  })
  const revalidate = useMutation({
    mutationFn: () => post<ServerTask>(teamPath(team, `/servers/${created!.server.id}/validate`)),
    onSuccess: (t) => setCreated((c) => c && { ...c, task: t.id }),
    onError: (e) => toast.error(errorText(e, "Validation impossible")),
  })
  const task = useServerTask(team, created?.server.id, created?.task)
  const server = useQuery({
    queryKey: ["team", team, "server", created?.server.id],
    queryFn: () => get<{ server: Server }>(teamPath(team, `/servers/${created!.server.id}`)).then((r) => r.server),
    enabled: !!created,
    refetchInterval: () => (task.data && task.data.status !== "queued" && task.data.status !== "running" ? false : 1500),
  })
  const finished = task.data && task.data.status !== "queued" && task.data.status !== "running"
  const command = key ? `mkdir -p ~/.ssh && echo "${key.public_key.trim()}" >> ~/.ssh/authorized_keys` : ""

  if (created)
    return (
      <div className="space-y-4">
        <ServerStatus server={server.data} task={task.data} />
        <TaskLog task={task.data} />
        <div className="flex justify-end gap-2">
          {finished && !server.data?.is_usable && (
            <Button variant="outline" loading={revalidate.isPending} onClick={() => revalidate.mutate()}>
              <RefreshCwIcon /> Relancer la validation
            </Button>
          )}
          <Button
            disabled={!finished || !server.data?.is_usable}
            onClick={() => {
              void qc.invalidateQueries({ queryKey: ["team", team, "servers"] })
              onReady(server.data!)
            }}
          >
            Continuer
          </Button>
        </div>
      </div>
    )

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        connect.mutate()
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Adresse IP ou domaine">
          <Input required className="font-mono" placeholder="203.0.113.10" value={f.ip} onChange={(e) => setF({ ...f, ip: e.target.value })} />
        </Field>
        <Field label="Nom" hint="Vide : l'adresse.">
          <Input placeholder="production-1" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
        </Field>
        <Field label="Utilisateur SSH">
          <Input className="font-mono" value={f.user} onChange={(e) => setF({ ...f, user: e.target.value })} />
        </Field>
        <Field label="Port SSH">
          <Input type="number" min={1} max={65535} className="font-mono" value={f.port} onChange={(e) => setF({ ...f, port: e.target.value })} />
        </Field>
      </div>

      <div className="space-y-3">
        <p className="text-sm font-medium">Clé SSH</p>
        {key ? (
          mode === "generate" ? (
            <div className="space-y-2 rounded-xl border border-border bg-muted p-4 text-sm">
              <p>Ajoutez cette clé publique au serveur (une fois connecté avec l'utilisateur ci-dessus) :</p>
              <div className="flex items-start gap-2 rounded-lg bg-black/70 p-3">
                <code className="flex-1 font-mono text-xs break-all text-zinc-100">{command}</code>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Copier la commande"
                  onClick={() => void navigator.clipboard.writeText(command).then(() => toast.success("Commande copiée"))}
                >
                  <CopyIcon />
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">Empreinte {key.fingerprint}</p>
            </div>
          ) : (
            <p className="rounded-xl border border-success/30 bg-success/10 p-3 text-sm">Clé importée · {key.fingerprint}</p>
          )
        ) : (
          <>
            <div className="grid gap-3 sm:grid-cols-2">
              <ChoiceCard
                icon={<SparklesIcon />}
                title="Générer une clé"
                text="Nodra crée une paire ed25519 ; vous ajoutez la clé publique au serveur."
                selected={mode === "generate"}
                onClick={() => setMode("generate")}
              />
              <ChoiceCard
                icon={<KeyRoundIcon />}
                title="Utiliser ma clé"
                text="Collez une clé privée existante (sans phrase de passe)."
                selected={mode === "paste"}
                onClick={() => setMode("paste")}
              />
            </div>
            {mode === "paste" && (
              <Field label="Clé privée">
                <Textarea rows={5} className="font-mono text-xs" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" value={pasted} onChange={(e) => setPasted(e.target.value)} />
              </Field>
            )}
            <Button type="button" variant="outline" loading={makeKey.isPending} disabled={mode === "paste" && !pasted.trim()} onClick={() => makeKey.mutate()}>
              <KeyRoundIcon /> {mode === "generate" ? "Générer la clé" : "Importer la clé"}
            </Button>
          </>
        )}
      </div>

      <div className="flex justify-end">
        <Button type="submit" disabled={!key || !f.ip.trim()} loading={connect.isPending}>
          Connecter et valider
        </Button>
      </div>
    </form>
  )
}
