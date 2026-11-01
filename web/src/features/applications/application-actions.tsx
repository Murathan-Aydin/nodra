import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { ChevronDownIcon, HammerIcon, PlayIcon, RotateCwIcon, SquareIcon } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { post } from "@/lib/api"
import type { Application, Deployment } from "@/lib/types"
import { LinksMenu } from "@/features/resources/resource-actions"
import { resourceApi, resourceKey, resourcePage, type ResourceRef } from "@/features/resources/resource"

type Kind = "deploy" | "force" | "restart" | "stop"

/** Actions d'une application (barre du haut) : Liens, Déployer / Redéployer ⌄ (redémarrer, reconstruire, arrêter). */
export function ApplicationActions({ resource, app }: { resource: ResourceRef; app: Application }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const running = /^(running|degraded|restarting)/.test(app.status)
  const action = useMutation({
    mutationFn: (kind: Kind) =>
      kind === "stop"
        ? post<{ approval_request?: unknown }>(resourceApi(resource, "/stop"))
        : kind === "restart"
          ? post<Deployment & { approval_request?: unknown }>(resourceApi(resource, "/restart"))
          : post<Deployment & { approval_request?: unknown }>(resourceApi(resource, "/deploy"), { force_rebuild: kind === "force" }),
    onSuccess: (r, kind) => {
      void qc.invalidateQueries({ queryKey: resourceKey(resource) })
      if (r && "approval_request" in r && r.approval_request) return void toast.info("En attente d'approbation")
      if (kind === "stop") return void toast.success("Arrêt lancé")
      toast.success(kind === "restart" ? "Redémarrage lancé" : "Déploiement lancé")
      // Comme Coolify : on suit le déploiement aussitôt.
      if (r && "id" in r && r.id) void navigate({ to: resourcePage(resource, `/deployments/${r.id}`) })
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <>
      <LinksMenu links={(app.domains ?? []).map((url) => ({ url }))} />
      <div className="flex items-center">
        <Button size="sm" className="rounded-r-none" loading={action.isPending} onClick={() => action.mutate("deploy")}>
          {running ? <RotateCwIcon /> : <PlayIcon />} {running ? "Redéployer" : "Déployer"}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm" className="rounded-l-none border-l border-white/15 px-1.5" disabled={action.isPending} aria-label="Plus d'actions">
              <ChevronDownIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-60">
            <DropdownMenuItem onSelect={() => action.mutate("restart")}>
              <RotateCwIcon /> Redémarrer (sans reconstruire)
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => action.mutate("force")}>
              <HammerIcon /> Reconstruire sans cache
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem disabled={!running} onSelect={() => action.mutate("stop")}>
              <SquareIcon /> Arrêter
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </>
  )
}
