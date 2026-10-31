import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { PlusIcon, ServerIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { relativeTime } from "@/lib/format"
import { usePermission } from "@/lib/permissions"
import { AddServerDialog } from "@/features/servers/add-server-dialog"
import { KeysPanel } from "@/features/servers/keys-panel"
import { serversQuery } from "@/features/servers/queries"

export const Route = createFileRoute("/t/$team/servers/")({ component: ServersPage })

function ServersPage() {
  const { team } = Route.useParams()
  const servers = useQuery(serversQuery(team))
  const canCreate = usePermission(team, "server.create")
  const [adding, setAdding] = useState(false)
  return (
    <>
      <PageHeader
        title="Serveurs"
        description="Machines pilotées par Nodra en SSH : Docker, proxy, ressources."
        actions={
          canCreate && (
            <Button onClick={() => setAdding(true)}>
              <PlusIcon /> Ajouter un serveur
            </Button>
          )
        }
      />
      <Tabs defaultValue="servers">
        <TabsList>
          <TabsTrigger value="servers">Serveurs</TabsTrigger>
          <TabsTrigger value="keys">Clés SSH</TabsTrigger>
        </TabsList>
        <TabsContent value="servers">
          {servers.isPending ? (
            <Skeleton className="h-40 w-full" />
          ) : !servers.data?.length ? (
            <EmptyState
              icon={ServerIcon}
              title="Aucun serveur"
              description="Ajoutez un serveur Linux joignable en SSH : Nodra vérifie la connexion et peut installer Docker."
              action={canCreate && <Button onClick={() => setAdding(true)}>Ajouter un serveur</Button>}
            />
          ) : (
            <Card>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Nom</TableHead>
                    <TableHead>Adresse</TableHead>
                    <TableHead>État</TableHead>
                    <TableHead>Proxy</TableHead>
                    <TableHead>Docker</TableHead>
                    <TableHead>Vu</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {servers.data.map((s) => (
                    <TableRow key={s.id}>
                      <TableCell>
                        <Link to="/t/$team/servers/$server" params={{ team, server: s.id }} className="font-medium hover:underline">
                          {s.name}
                        </Link>
                        {s.description && <p className="text-xs text-muted-foreground">{s.description}</p>}
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        {s.user}@{s.ip}:{s.port}
                      </TableCell>
                      <TableCell>
                        {!s.is_usable ? (
                          <Badge variant="warning">À valider</Badge>
                        ) : (
                          <StatusBadge status={s.is_reachable ? "reachable" : "unreachable"} />
                        )}
                      </TableCell>
                      <TableCell>
                        {s.proxy.type === "none" ? (
                          <span className="text-xs text-muted-foreground">Aucun</span>
                        ) : (
                          <span className="flex items-center gap-1.5 text-xs">
                            <span className="capitalize">{s.proxy.type}</span>
                            <StatusBadge status={s.proxy.status} />
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">{s.docker_version || "—"}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{relativeTime(s.last_seen_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </TabsContent>
        <TabsContent value="keys">
          <KeysPanel team={team} />
        </TabsContent>
      </Tabs>
      <AddServerDialog team={team} open={adding} onOpenChange={setAdding} />
    </>
  )
}
