import { createFileRoute } from "@tanstack/react-router"
import { useInfiniteQuery, useQuery } from "@tanstack/react-query"
import { useDeferredValue, useState } from "react"
import { DownloadIcon, LockIcon, ScrollTextIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { buildURL, get, teamPath } from "@/lib/api"
import { dateTime, relativeTime } from "@/lib/format"
import { usePermission } from "@/lib/permissions"
import type { AuditEvent } from "@/lib/types"
import { membersQuery } from "@/features/team/queries"

export const Route = createFileRoute("/t/$team/audit/")({ component: Audit })

const TARGETS = ["application", "database", "service", "server", "project", "environment", "team", "resource", "source", "s3_storage", "approval"]

function Audit() {
  const { team } = Route.useParams()
  const allowed = usePermission(team, "team.audit.read")
  const members = useQuery(membersQuery(team))
  const [f, setF] = useState({ action: "", actor: "all", target_type: "all", since: "", until: "" })
  const action = useDeferredValue(f.action.trim())
  const filters = {
    action: action || undefined,
    actor: f.actor === "all" ? undefined : f.actor,
    target_type: f.target_type === "all" ? undefined : f.target_type,
    since: f.since ? new Date(f.since).toISOString() : undefined,
    until: f.until ? new Date(f.until).toISOString() : undefined,
  }
  const events = useInfiniteQuery({
    queryKey: ["team", team, "audit", filters],
    queryFn: ({ pageParam }) => get<{ events: AuditEvent[]; next_before: number | null }>(teamPath(team, "/audit"), { ...filters, before: pageParam, limit: 100 }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.next_before ?? undefined,
    enabled: allowed,
  })
  const [open, setOpen] = useState<AuditEvent | null>(null)
  const rows = events.data?.pages.flatMap((p) => p.events) ?? []

  if (!allowed)
    return (
      <>
        <PageHeader title="Journal d'audit" />
        <EmptyState icon={LockIcon} title="Accès refusé" description="La permission « Consulter le journal d'audit » est nécessaire." />
      </>
    )
  return (
    <>
      <PageHeader
        title="Journal d'audit"
        description="Toutes les actions de l'équipe et de ses sous-équipes (ajout seul, conservé après suppression)."
        actions={
          <Button variant="outline" asChild>
            <a href={buildURL(teamPath(team, "/audit/export"), filters)} download>
              <DownloadIcon /> Exporter (CSV)
            </a>
          </Button>
        }
      />
      <div className="mb-4 flex flex-wrap gap-2">
        <Input className="w-56" placeholder="Action (préfixe, ex. resource.)" value={f.action} onChange={(e) => setF({ ...f, action: e.target.value })} aria-label="Action" />
        <Select value={f.actor} onValueChange={(v) => setF({ ...f, actor: v })}>
          <SelectTrigger className="w-56" aria-label="Auteur">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Tous les auteurs</SelectItem>
            {members.data?.map((m) => (
              <SelectItem key={m.user_id} value={m.user_id}>
                {m.name || m.email}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={f.target_type} onValueChange={(v) => setF({ ...f, target_type: v })}>
          <SelectTrigger className="w-44" aria-label="Type de cible">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Toutes les cibles</SelectItem>
            {TARGETS.map((t) => (
              <SelectItem key={t} value={t}>
                {t}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input type="datetime-local" className="w-52" value={f.since} onChange={(e) => setF({ ...f, since: e.target.value })} aria-label="Depuis" />
        <Input type="datetime-local" className="w-52" value={f.until} onChange={(e) => setF({ ...f, until: e.target.value })} aria-label="Jusqu'à" />
      </div>
      {events.isPending ? (
        <Skeleton className="h-64" />
      ) : !rows.length ? (
        <EmptyState icon={ScrollTextIcon} title="Aucun événement" />
      ) : (
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Date</TableHead>
                <TableHead>Auteur</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Cible</TableHead>
                <TableHead>IP</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((e) => (
                <TableRow key={e.id} className="cursor-pointer" onClick={() => setOpen(e)}>
                  <TableCell className="whitespace-nowrap text-muted-foreground" title={dateTime(e.created_at)}>
                    {relativeTime(e.created_at)}
                  </TableCell>
                  <TableCell>
                    {e.actor_email || "système"}
                    {e.actor_token_id && <span className="ml-1 text-xs text-muted-foreground">(jeton)</span>}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{e.action}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {e.target_type} {e.target_id && <span className="font-mono">{e.target_id.slice(0, 8)}</span>}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{e.ip}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
      {events.hasNextPage && (
        <div className="mt-4 flex justify-center">
          <Button variant="outline" loading={events.isFetchingNextPage} onClick={() => void events.fetchNextPage()}>
            Plus ancien
          </Button>
        </div>
      )}
      <Dialog open={!!open} onOpenChange={(o) => !o && setOpen(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="font-mono">{open?.action}</DialogTitle>
            <DialogDescription>
              {open && `${dateTime(open.created_at)} · ${open.actor_email || "système"} · ${open.ip}`}
            </DialogDescription>
          </DialogHeader>
          {open && (
            <div className="space-y-2 text-sm">
              <p>
                Cible : {open.target_type} <code className="font-mono text-xs">{open.target_id}</code>
              </p>
              {open.user_agent && <p className="truncate text-xs text-muted-foreground">{open.user_agent}</p>}
              <pre className="max-h-96 overflow-auto rounded-lg bg-muted p-3 font-mono text-xs">{JSON.stringify(open.metadata, null, 2)}</pre>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
