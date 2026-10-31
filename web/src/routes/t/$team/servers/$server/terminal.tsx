import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { LockIcon, PlayIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { WebTerminal } from "@/components/terminal"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { ServerTask } from "@/lib/types"

export const Route = createFileRoute("/t/$team/servers/$server/terminal")({ component: ServerTerminal })

function ServerTerminal() {
  const { team, server } = Route.useParams()
  const navigate = useNavigate()
  const allowed = usePermission(team, "server.terminal", { type: "server", id: server })
  const [command, setCommand] = useState("")
  const run = useApiMutation((cmd: string) => post<ServerTask>(teamPath(team, `/servers/${server}/commands`), { command: cmd }), {
    onSuccess: (task) => void navigate({ to: "/t/$team/servers/$server/tasks", params: { team, server }, search: { task: task.id } }),
  })
  if (!allowed) return <EmptyState icon={LockIcon} title="Accès refusé" description="La permission « Ouvrir un terminal sur les serveurs » est nécessaire." />
  return (
    <div className="space-y-4">
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (command.trim()) run.mutate(command.trim())
        }}
      >
        <Input className="font-mono" placeholder="Commande ponctuelle (journalisée), ex. df -h" value={command} onChange={(e) => setCommand(e.target.value)} />
        <Button type="submit" variant="outline" loading={run.isPending}>
          <PlayIcon /> Exécuter
        </Button>
      </form>
      <WebTerminal path={teamPath(team, `/servers/${server}/terminal`)} />
      <p className="text-xs text-muted-foreground">Shell de l'utilisateur SSH du serveur. L'ouverture est enregistrée dans le journal d'audit.</p>
    </div>
  )
}
