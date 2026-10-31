import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { LockIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { LogStream } from "@/components/log-stream"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { usePermission } from "@/lib/permissions"
import { containersQuery } from "./containers"

/** Logs suivis des conteneurs d'une ressource (tous, ou un conteneur précis). */
export function LogsPanel({ team, base, resource, service }: { team: string; base: string; resource: string; service?: string }) {
  const allowed = usePermission(team, "resource.logs.read", { type: "resource", id: resource })
  const containers = useQuery({ ...containersQuery(team, resource, base), enabled: allowed })
  const [container, setContainer] = useState("all")
  const [tail, setTail] = useState("200")
  if (!allowed) return <EmptyState icon={LockIcon} title="Accès refusé" description="La permission « Lire les logs » est nécessaire." />
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        <Select value={container} onValueChange={setContainer}>
          <SelectTrigger className="w-72">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Tous les conteneurs</SelectItem>
            {containers.data?.map((c) => (
              <SelectItem key={c.name} value={c.name}>
                {c.service ? `${c.service} · ` : ""}
                {c.name} ({c.state})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={tail} onValueChange={setTail}>
          <SelectTrigger className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {["100", "200", "1000", "5000"].map((n) => (
              <SelectItem key={n} value={n}>
                {n} dernières lignes
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <LogStream
        key={`${container}-${tail}`}
        path={`${base}/logs/stream`}
        query={{ tail, container: container === "all" ? undefined : container, app: service }}
        filename="logs.txt"
      />
    </div>
  )
}
