import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { ArrowRightIcon, CheckCircle2Icon, CloudIcon, FolderKanbanIcon, RotateCcwIcon, ServerIcon, ContainerIcon } from "lucide-react"
import { toast } from "sonner"
import { AuthShell } from "@/components/auth-shell"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ApiError, get, post, teamPath } from "@/lib/api"
import { instanceQuery, teamQueries, teamsQuery } from "@/lib/queries"
import { requireSession } from "@/lib/session"
import type { Project, Server } from "@/lib/types"
import { LocalhostConnection, RemoteConnection } from "@/features/onboarding/connection"
import { ChoiceCard, FooterActions, Stepper } from "@/features/onboarding/ui"

type Step = "welcome" | "server" | "connection" | "project" | "done"
type Search = { step?: Step; type?: "localhost" | "remote"; server?: string; project?: string }

const STEPS: Step[] = ["welcome", "server", "connection", "project", "done"]

// Configuration guidée après la création du compte : serveur, connexion, premier projet.
export const Route = createFileRoute("/onboarding")({
  validateSearch: (s: Record<string, unknown>): Search => ({
    step: STEPS.includes(s.step as Step) ? (s.step as Step) : undefined,
    type: s.type === "localhost" || s.type === "remote" ? s.type : undefined,
    server: typeof s.server === "string" ? s.server : undefined,
    project: typeof s.project === "string" ? s.project : undefined,
  }),
  // Accessible une seule fois : à l'administrateur, tant qu'elle n'a été ni terminée ni passée.
  beforeLoad: async ({ context }) => {
    const me = await requireSession(context.queryClient, "/onboarding")
    const instance = await context.queryClient.fetchQuery(instanceQuery)
    if (!instance.onboarding || !me.user.is_instance_admin) throw redirect({ to: "/" })
  },
  component: Onboarding,
})

function Onboarding() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const step = search.step ?? "welcome"
  const go = (s: Search) => void navigate({ search: (prev) => ({ ...prev, ...s }) })
  const teams = useQuery(teamsQuery)
  const team = teams.data?.find((t) => t.personal)?.id ?? teams.data?.[0]?.id
  const finish = useFinish()

  return (
    <AuthShell wide>
      {step !== "welcome" && <Stepper current={step === "server" ? 0 : step === "connection" ? 1 : 2} />}
      <div className="mt-6 w-full">
        {step === "welcome" && <Welcome onStart={() => go({ step: "server" })} />}
        {step === "server" && team && <ChooseServer team={team} selected={search.type} onChoose={(type) => go({ step: "connection", type })} />}
        {step === "connection" && team && (
          <Panel
            title={search.type === "localhost" ? "Cette machine" : "Connexion SSH"}
            description={
              search.type === "localhost"
                ? "Nodra vérifie l'accès à l'hôte sur lequel il est installé."
                : "Nodra se connecte en SSH pour vérifier le système et installer ce qui manque."
            }
          >
            {search.type === "localhost" ? (
              <LocalhostConnection team={team} onReady={(s) => go({ step: "project", server: s.id })} />
            ) : (
              <RemoteConnection team={team} onReady={(s) => go({ step: "project", server: s.id })} />
            )}
          </Panel>
        )}
        {step === "project" && team && <CreateProject team={team} onCreated={(p) => go({ step: "done", project: p.id })} />}
        {step === "done" && team && <Done team={team} server={search.server} project={search.project} />}
      </div>
      {step !== "done" && (
        <FooterActions>
          <Button variant="ghost" size="sm" loading={finish.isPending} onClick={() => finish.mutate()}>
            <ArrowRightIcon /> Passer la configuration
          </Button>
          {step !== "welcome" && (
            <Button variant="ghost" size="sm" onClick={() => void navigate({ search: {} })}>
              <RotateCcwIcon /> Recommencer
            </Button>
          )}
        </FooterActions>
      )}
    </AuthShell>
  )
}

/** Termine (ou passe) la configuration guidée : elle ne sera plus jamais proposée. */
function useFinish() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  return useMutation({
    mutationFn: () => post("/admin/onboarding/complete"),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["instance"] })
      void navigate({ to: "/" })
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Action impossible"),
  })
}

function Panel({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <section className="glass-panel-strong rounded-[var(--radius-sheet)] p-7">
      <header className="mb-6 space-y-1">
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </header>
      {children}
    </section>
  )
}

const PLAN = [
  { icon: ServerIcon, title: "Connexion au serveur", text: "Accès SSH à la machine qui hébergera vos ressources." },
  { icon: ContainerIcon, title: "Environnement Docker", text: "Vérification du système et installation de Docker si besoin." },
  { icon: FolderKanbanIcon, title: "Premier projet", text: "Un projet et son environnement de production." },
]

function Welcome({ onStart }: { onStart: () => void }) {
  return (
    <div className="flex flex-col items-center text-center">
      <h1 className="text-3xl font-semibold tracking-tight">Bienvenue sur Nodra</h1>
      <p className="mt-2 text-muted-foreground">Connectez votre premier serveur et déployez en quelques minutes.</p>
      <section className="glass-panel-strong mt-8 w-full rounded-[var(--radius-sheet)] p-2 text-left">
        <p className="px-4 pt-3 pb-2 text-sm font-medium">Ce que vous allez configurer</p>
        <ul className="divide-y divide-border">
          {PLAN.map(({ icon: Icon, title, text }) => (
            <li key={title} className="flex items-center gap-4 px-4 py-3.5">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-border bg-muted">
                <Icon className="size-5" />
              </span>
              <span>
                <span className="block font-medium">{title}</span>
                <span className="block text-sm text-muted-foreground">{text}</span>
              </span>
            </li>
          ))}
        </ul>
      </section>
      <Button size="lg" className="mt-8 min-w-48" onClick={onStart}>
        Commencer
      </Button>
    </div>
  )
}

function ChooseServer({ team, selected, onChoose }: { team: string; selected?: string; onChoose: (t: "localhost" | "remote") => void }) {
  const instance = useQuery(instanceQuery).data
  // Une équipe = un serveur : s'il existe déjà (hôte créé par install.sh), c'est lui qu'on valide.
  const existing = useQuery(teamQueries.servers(team)).data?.[0]
  return (
    <Panel title="Où déployer ?" description="Choisissez le serveur qui hébergera vos applications et bases de données. Vous pourrez en ajouter d'autres ensuite.">
      <div className="grid gap-4 sm:grid-cols-2">
        <ChoiceCard
          icon={<ServerIcon />}
          title="Cette machine"
          text={
            instance?.localhost
              ? "Déployer sur le serveur où tourne Nodra. Idéal pour démarrer ou un seul serveur."
              : "Indisponible : Nodra n'a pas été installé avec install.sh sur cette machine."
          }
          selected={selected === "localhost"}
          disabled={!instance?.localhost || (!!existing && !existing.is_localhost)}
          onClick={() => onChoose("localhost")}
        />
        <ChoiceCard
          icon={<CloudIcon />}
          title="Adresse IP ou domaine"
          text={existing ? "Indisponible : l'équipe a déjà son serveur (un seul par équipe)." : "Connecter en SSH un serveur que vous gérez déjà (VPS, machine dédiée…)."}
          selected={selected === "remote"}
          disabled={!!existing}
          onClick={() => onChoose("remote")}
        />
      </div>
    </Panel>
  )
}

function CreateProject({ team, onCreated }: { team: string; onCreated: (p: Project) => void }) {
  const [name, setName] = useState("Mon premier projet")
  const create = useMutation({
    mutationFn: () => post<Project>(teamPath(team, "/projects"), { name: name.trim() }),
    onSuccess: onCreated,
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Création impossible"),
  })
  return (
    <Panel title="Premier projet" description="Un projet regroupe vos applications, bases de données et services, par environnement.">
      <form
        className="flex flex-col gap-4 sm:flex-row sm:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate()
        }}
      >
        <Field label="Nom du projet" className="flex-1">
          <Input required value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Button type="submit" loading={create.isPending} disabled={!name.trim()}>
          Créer le projet
        </Button>
      </form>
    </Panel>
  )
}

function Done({ team, server, project }: { team: string; server?: string; project?: string }) {
  const sv = useQuery({
    queryKey: ["team", team, "server", server],
    queryFn: () => get<{ server: Server }>(teamPath(team, `/servers/${server}`)).then((r) => r.server),
    enabled: !!server,
  })
  const pj = useQuery({
    queryKey: ["team", team, "project", project],
    queryFn: () => get<{ project: Project }>(teamPath(team, `/projects/${project}`)).then((r) => r.project),
    enabled: !!project,
  })
  const items = [
    sv.data && { title: `Serveur : ${sv.data.name}`, text: sv.data.is_localhost ? "Cette machine" : `${sv.data.user}@${sv.data.ip}:${sv.data.port}` },
    pj.data && { title: `Projet : ${pj.data.name}`, text: "Environnement de production prêt" },
    sv.data?.docker_version && { title: "Docker", text: `Version ${sv.data.docker_version}, installé et démarré` },
  ].filter(Boolean) as { title: string; text: string }[]
  const finish = useFinish()
  return (
    <div className="flex flex-col items-center text-center">
      <span className="flex size-16 items-center justify-center rounded-2xl border border-success/30 bg-success/15">
        <CheckCircle2Icon className="size-8 text-success" />
      </span>
      <h1 className="mt-5 text-3xl font-semibold tracking-tight">Configuration terminée</h1>
      <p className="mt-2 text-muted-foreground">Votre serveur est connecté. Vous pouvez déployer votre première ressource.</p>
      <section className="glass-panel-strong mt-8 w-full rounded-[var(--radius-sheet)] p-2 text-left">
        <p className="px-4 pt-3 pb-2 text-xs font-medium tracking-wider text-muted-foreground uppercase">Ce qui est configuré</p>
        <ul className="divide-y divide-border">
          {items.map((i) => (
            <li key={i.title} className="flex items-center gap-4 px-4 py-3.5">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-success/30 bg-success/10">
                <CheckCircle2Icon className="size-5 text-success" />
              </span>
              <span>
                <span className="block font-medium">{i.title}</span>
                <span className="block text-sm text-muted-foreground">{i.text}</span>
              </span>
            </li>
          ))}
        </ul>
      </section>
      <Button size="lg" className="mt-8 min-w-48" loading={finish.isPending} onClick={() => finish.mutate()}>
        Accéder à Nodra
      </Button>
    </div>
  )
}
