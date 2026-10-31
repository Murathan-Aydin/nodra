import { createFileRoute, Link, Outlet, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ChevronDownIcon, DownloadIcon, PackageIcon, PowerIcon, ShieldCheckIcon, SparklesIcon, WrenchIcon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { PageHeader } from "@/components/page-header"
import { TAB_ACTIVE, TAB_LINK, TabNav } from "@/components/tab-nav"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { ServerTask } from "@/lib/types"
import { serverQuery } from "@/features/servers/queries"

export const Route = createFileRoute("/t/$team/servers/$server")({ component: ServerLayout })

const TABS = [
  { to: "/t/$team/servers/$server", label: "Vue d'ensemble", exact: true },
  { to: "/t/$team/servers/$server/tasks", label: "Tâches" },
  { to: "/t/$team/servers/$server/terminal", label: "Terminal" },
  { to: "/t/$team/servers/$server/proxy", label: "Proxy" },
  { to: "/t/$team/servers/$server/settings", label: "Réglages" },
] as const

function ServerLayout() {
  const { team, server } = Route.useParams()
  const navigate = useNavigate()
  const { data, isPending } = useQuery(serverQuery(team, server))
  const scope = { type: "server", id: server }
  const canUpdate = usePermission(team, "server.update", scope)
  const [rebooting, setRebooting] = useState(false)
  const action = useApiMutation((path: string) => post<ServerTask>(teamPath(team, `/servers/${server}${path}`)), {
    invalidate: [["team", team, "server", server]],
    onSuccess: (task) => {
      setRebooting(false)
      if (task?.id) void navigate({ to: "/t/$team/servers/$server/tasks", params: { team, server }, search: { task: task.id } })
    },
  })
  const sv = data?.server
  return (
    <>
      {isPending || !sv ? (
        <Skeleton className="mb-6 h-12 w-80" />
      ) : (
        <PageHeader
          title={
            <span className="flex items-center gap-3">
              {sv.name}
              {!sv.is_usable ? <Badge variant="warning">À valider</Badge> : <StatusBadge status={sv.is_reachable ? "reachable" : "unreachable"} />}
            </span>
          }
          description={
            <span className="font-mono">
              {sv.user}@{sv.ip}:{sv.port}
              {sv.description && <span className="font-sans"> · {sv.description}</span>}
            </span>
          }
          actions={
            canUpdate && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline">
                    Actions <ChevronDownIcon />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                  <DropdownMenuItem onSelect={() => action.mutate("/validate")}>
                    <ShieldCheckIcon /> Valider la connexion
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => action.mutate("/install-docker")}>
                    <DownloadIcon /> Installer Docker
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => action.mutate("/docker-cleanup")}>
                    <SparklesIcon /> Nettoyage Docker
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onSelect={() => action.mutate("/packages/check")}>
                    <PackageIcon /> Rechercher des mises à jour
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => action.mutate("/packages/update")}>
                    <WrenchIcon /> Mettre à jour les paquets
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onSelect={() => setRebooting(true)}>
                    <PowerIcon /> Redémarrer le serveur
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )
          }
        />
      )}
      <TabNav>
        {TABS.map((t) => (
          <Link
            key={t.to}
            to={t.to}
            params={{ team, server }}
            activeOptions={{ exact: "exact" in t }}
            className={TAB_LINK}
            activeProps={TAB_ACTIVE}
          >
            {t.label}
          </Link>
        ))}
      </TabNav>
      <Outlet />
      <ConfirmDialog
        open={rebooting}
        onOpenChange={setRebooting}
        title="Redémarrer le serveur ?"
        description="Les applications hébergées seront indisponibles pendant le redémarrage."
        confirmLabel="Redémarrer"
        destructive
        loading={action.isPending}
        onConfirm={() => action.mutate("/reboot")}
      />
    </>
  )
}
