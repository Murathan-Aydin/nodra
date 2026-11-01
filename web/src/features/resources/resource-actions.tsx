import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ChevronDownIcon, ExternalLinkIcon, PlayIcon, RotateCwIcon, SquareIcon } from "lucide-react"
import type { ReactNode } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { ApiError, post } from "@/lib/api"
import type { Service, ServiceApp } from "@/lib/types"
import { resourceApi, resourceKey, type ResourceRef } from "./resource"

type Action = "start" | "restart" | "stop"

const DONE: Record<Action, string> = {
  start: "Démarrage lancé",
  restart: "Redémarrage lancé",
  stop: "Arrêt lancé",
}

/** Domaine → lien absolu (https par défaut). */
const href = (domain: string) => (/^https?:\/\//.test(domain) ? domain : `https://${domain}`)

/** Menu « Liens » : domaines publics de la ressource. */
export function LinksMenu({ links }: { links: { label?: string; url: string }[] }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="ghost" disabled={links.length === 0}>
          <ExternalLinkIcon /> Liens <ChevronDownIcon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        {links.map(({ label, url }) => (
          <DropdownMenuItem key={`${label}-${url}`} asChild>
            <a href={href(url)} target="_blank" rel="noreferrer" title={url}>
              <ExternalLinkIcon />
              <span className="truncate">{url}</span>
              {label && <span className="ml-auto shrink-0 text-[10px] font-normal text-muted-foreground">{label}</span>}
            </a>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/**
 * Bouton principal Démarrer / Redémarrer et menu ⌄ (Arrêter + `extra`), comme « Restart ⌄ » du fork.
 * Pendant un démarrage ou un redémarrage, le bouton est désactivé.
 */
export function LifecycleActions({ resource, status, startLabel = "Démarrer", extra }: { resource: ResourceRef; status: string; startLabel?: string; extra?: ReactNode }) {
  const qc = useQueryClient()
  const running = status.startsWith("running") || status.startsWith("degraded")
  const busy = /^(starting|restarting)/.test(status)
  const action = useMutation({
    mutationFn: (kind: Action) => post<{ approval_request?: unknown }>(resourceApi(resource, `/${kind}`)),
    onSuccess: (r, kind) => {
      // 202 avec demande d'approbation : la tâche ne partira qu'après validation.
      if (r?.approval_request) toast.info("En attente d'approbation", { description: `${DONE[kind]} après validation.` })
      else toast.success(DONE[kind])
      void qc.invalidateQueries({ queryKey: resourceKey(resource) })
      void qc.invalidateQueries({ queryKey: ["team", resource.team, "resources"] })
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Action impossible"),
  })
  return (
    <div className="flex items-center">
      <Button size="sm" className="rounded-r-none" loading={action.isPending} disabled={busy} onClick={() => action.mutate(running ? "restart" : "start")}>
        {running ? <RotateCwIcon /> : <PlayIcon />} {busy ? "En cours…" : running ? "Redémarrer" : startLabel}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" className="rounded-l-none border-l border-white/15 px-1.5" disabled={action.isPending} aria-label="Plus d'actions">
            <ChevronDownIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem disabled={!running && !busy} onSelect={() => action.mutate("stop")}>
            <SquareIcon /> Arrêter
          </DropdownMenuItem>
          {extra}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

/** Actions d'un service : liens des domaines, démarrer/redémarrer, arrêter (barre du haut). */
export function ServiceActions({ team, service, apps }: { team: string; service: Service; apps: ServiceApp[] }) {
  const links = apps.flatMap((app) => (app.domains ?? []).map((url) => ({ label: app.name, url })))
  return (
    <>
      <LinksMenu links={links} />
      <LifecycleActions resource={{ team, kind: "service", id: service.id }} status={service.status} />
    </>
  )
}
