import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { HistoryIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { TaskDialog, taskLabel } from "@/components/task-output"
import { Card } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { dateTime, duration, relativeTime } from "@/lib/format"
import type { ServerTask } from "@/lib/types"
import { svcTasksQuery } from "@/features/services/queries"

export const Route = createFileRoute("/t/$team/services/$svc/history")({ component: ServiceHistory })

function ServiceHistory() {
  const { team, svc } = Route.useParams()
  const tasks = useQuery(svcTasksQuery(team, svc))
  const [open, setOpen] = useState<ServerTask | null>(null)
  if (!tasks.data?.length) return <EmptyState icon={HistoryIcon} title="Aucune action" description="Démarrages, arrêts et redémarrages (du service ou d'un conteneur) apparaissent ici." />
  return (
    <Card>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Action</TableHead>
            <TableHead>Statut</TableHead>
            <TableHead>Date</TableHead>
            <TableHead className="text-right">Durée</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {tasks.data.map((t) => (
            <TableRow key={t.id} className="cursor-pointer" onClick={() => setOpen(t)}>
              <TableCell className="font-medium">{taskLabel(t.kind)}</TableCell>
              <TableCell>
                <StatusBadge status={t.status} />
              </TableCell>
              <TableCell className="text-muted-foreground" title={dateTime(t.created_at)}>
                {relativeTime(t.created_at)}
              </TableCell>
              <TableCell className="text-right text-muted-foreground tabular-nums">{t.started_at ? duration(t.started_at, t.finished_at) : "—"}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <TaskDialog team={team} task={open} onOpenChange={(o) => !o && setOpen(null)} />
    </Card>
  )
}
