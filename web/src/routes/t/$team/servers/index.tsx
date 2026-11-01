import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { PlusIcon, ServerIcon } from "lucide-react"
import { EmptyPanel, PageHeader } from "@/components/page"
import { Pill } from "@/components/task-status"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { RemoteConnection } from "@/features/onboarding/connection"
import { serverState } from "@/features/servers/server"
import { relativeTime } from "@/lib/format"
import { teamQueries } from "@/lib/queries"

export const Route = createFileRoute("/t/$team/servers/")({ component: Servers })

/** Serveurs de l'équipe (un seul par équipe) et ajout d'un serveur par SSH. */
function Servers() {
  const { team } = Route.useParams()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const servers = useQuery(teamQueries.servers(team))
  const [adding, setAdding] = useState(false)
  const none = servers.data?.length === 0
  return (
    <>
      <PageHeader
        title="Serveurs"
        description="Le serveur de l'équipe, sur lequel vos ressources sont déployées."
        actions={
          none && (
            <Button size="sm" onClick={() => setAdding(true)}>
              <PlusIcon /> Ajouter
            </Button>
          )
        }
      />
      {servers.isPending ? (
        <Skeleton className="h-32 rounded-2xl" />
      ) : none ? (
        <EmptyPanel
          icon={<ServerIcon />}
          title="Aucun serveur"
          text="Ajoutez le serveur sur lequel l'équipe déploiera ses ressources (connexion SSH)."
          action={
            <Button size="sm" onClick={() => setAdding(true)}>
              <PlusIcon /> Ajouter un serveur
            </Button>
          }
        />
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {servers.data?.map((s) => {
            const st = serverState(s)
            return (
              <Link key={s.id} to="/t/$team/servers/$server" params={{ team, server: s.id }} className="glass-panel group block rounded-2xl p-5 transition-all hover:-translate-y-0.5 hover:border-primary/50">
                <div className="flex items-start gap-3">
                  <span className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-border bg-muted text-muted-foreground">
                    <ServerIcon className="size-5" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-semibold">{s.name}</p>
                    <p className="truncate font-mono text-xs text-muted-foreground">
                      {s.user}@{s.ip}:{s.port}
                    </p>
                    {s.description && <p className="mt-1 truncate text-sm text-muted-foreground">{s.description}</p>}
                  </div>
                  <Pill label={st.label} dot={st.dot} />
                </div>
                <p className="mt-4 text-xs text-muted-foreground">
                  {s.os_id ? `${s.os_id} ${s.os_version}` : "Système inconnu"} · {s.docker_version ? `Docker ${s.docker_version}` : "Docker absent"} · vu {relativeTime(s.last_seen_at)}
                </p>
              </Link>
            )
          })}
        </div>
      )}
      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Ajouter un serveur</DialogTitle>
            <DialogDescription>Nodra s'y connecte en SSH, vérifie le système et Docker, puis démarre le proxy.</DialogDescription>
          </DialogHeader>
          <RemoteConnection
            team={team}
            onReady={(s) => {
              setAdding(false)
              void qc.invalidateQueries({ queryKey: ["team", team, "servers"] })
              void navigate({ to: "/t/$team/servers/$server", params: { team, server: s.id } })
            }}
          />
        </DialogContent>
      </Dialog>
    </>
  )
}
