import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { LockIcon, TerminalIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { WebTerminal } from "@/components/terminal"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { usePermission } from "@/lib/permissions"
import { containersQuery } from "./containers"

/** Terminal dans un conteneur en cours d'exécution de la ressource. */
export function TerminalPanel({ team, base, resource }: { team: string; base: string; resource: string }) {
  const allowed = usePermission(team, "resource.terminal", { type: "resource", id: resource })
  const containers = useQuery({ ...containersQuery(team, resource, base), enabled: allowed })
  const running = containers.data?.filter((c) => c.state === "running") ?? []
  const [container, setContainer] = useState("")
  const [open, setOpen] = useState(false)
  if (!allowed) return <EmptyState icon={LockIcon} title="Accès refusé" description="La permission « Ouvrir un terminal dans les conteneurs » est nécessaire." />
  if (containers.isSuccess && !running.length) return <EmptyState icon={TerminalIcon} title="Aucun conteneur en cours d'exécution" description="Démarrez ou déployez la ressource d'abord." />
  const selected = container || (running.length === 1 ? running[0].name : "")
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        {running.length > 1 && (
          <Select value={container} onValueChange={setContainer}>
            <SelectTrigger className="w-72">
              <SelectValue placeholder="Choisir un conteneur" />
            </SelectTrigger>
            <SelectContent>
              {running.map((c) => (
                <SelectItem key={c.name} value={c.name}>
                  {c.service ? `${c.service} · ` : ""}
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {!open && (
          <Button disabled={!selected} onClick={() => setOpen(true)}>
            <TerminalIcon /> Ouvrir le terminal
          </Button>
        )}
      </div>
      {open && selected && <WebTerminal key={selected} path={`${base}/terminal`} query={{ container: selected }} />}
      <p className="text-xs text-muted-foreground">Shell (bash si disponible) dans le conteneur ; l'ouverture est enregistrée dans le journal d'audit.</p>
    </div>
  )
}
