import { useInfiniteQuery } from "@tanstack/react-query"
import { useState } from "react"
import { ChevronRightIcon, DownloadIcon } from "lucide-react"
import { PageHeader, SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { API_BASE, get } from "@/lib/api"
import { dateTime, downloadUrl, relativeTime } from "@/lib/format"
import { cn } from "@/lib/utils"

type Event = { id: number; actor_email: string | null; actor_token_id: string | null; action: string; target_type: string; target_id: string; ip: string; metadata: Record<string, unknown> | null; created_at: string }

/** Journal d'audit (équipe ou instance) : filtres, pagination par curseur, export CSV. */
export function AuditLog({ base, title = "Journal d'audit", scopeParam }: { base: string; title?: string; scopeParam?: string }) {
  const [action, setAction] = useState("")
  const [target, setTarget] = useState("")
  const params = new URLSearchParams({ limit: "50" })
  if (action) params.set("action", action)
  if (target) params.set("target_type", target)
  if (scopeParam) params.set("scope", scopeParam)
  const q = useInfiniteQuery({
    queryKey: ["audit", base, action, target],
    queryFn: ({ pageParam }) => get<{ events: Event[]; next_before: number | null }>(`${base}?${params}${pageParam ? `&before=${pageParam}` : ""}`),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_before ?? undefined,
  })
  const events = q.data?.pages.flatMap((p) => p.events) ?? []
  return (
    <>
      <PageHeader
        title={title}
        description="En ajout seul : les événements ne peuvent être ni modifiés ni supprimés."
        actions={
          <Button size="sm" variant="outline" onClick={() => downloadUrl(`${API_BASE}${base}/export?${params}`, "audit.csv")}>
            <DownloadIcon /> Export CSV
          </Button>
        }
      />
      <div className="mb-4 flex flex-wrap gap-2">
        <Input value={action} onChange={(e) => setAction(e.target.value)} placeholder="Action (préfixe, ex. service.)" className="max-w-xs" />
        <Input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="Type de cible (server, service…)" className="max-w-xs" />
      </div>
      <SettingsCard title="Événements" bodyClassName="p-0">
        {events.map((e) => (
          <EventRow key={e.id} e={e} />
        ))}
        {events.length === 0 && !q.isPending && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucun événement.</p>}
        {q.hasNextPage && (
          <div className="p-3 text-center">
            <Button size="sm" variant="ghost" loading={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
              Plus d'événements
            </Button>
          </div>
        )}
      </SettingsCard>
    </>
  )
}

function EventRow({ e }: { e: Event }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="border-b border-border last:border-b-0">
      <button type="button" onClick={() => setOpen(!open)} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-foreground/[0.03]">
        <ChevronRightIcon className={cn("size-4 shrink-0 text-muted-foreground transition-transform", open && "rotate-90")} />
        <span className="w-56 shrink-0 truncate font-mono text-xs">{e.action}</span>
        <span className="min-w-0 flex-1 truncate text-sm">{e.actor_email ?? (e.actor_token_id ? "Jeton API" : "Système")}</span>
        <span className="hidden truncate text-xs text-muted-foreground md:inline">
          {e.target_type} {e.target_id?.slice(0, 8)}
        </span>
        <span className="w-28 shrink-0 text-right text-xs text-muted-foreground" title={dateTime(e.created_at)}>
          {relativeTime(e.created_at)}
        </span>
      </button>
      {open && (
        <pre className="border-t border-border bg-black/30 px-4 py-3 font-mono text-xs whitespace-pre-wrap">
          {JSON.stringify({ cible: `${e.target_type} ${e.target_id}`, ip: e.ip, date: dateTime(e.created_at), ...e.metadata }, null, 2)}
        </pre>
      )}
    </div>
  )
}
