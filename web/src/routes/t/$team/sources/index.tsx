import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { GitBranchIcon, PlusIcon } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { usePermission } from "@/lib/permissions"
import { CreateSourceDialog } from "@/features/sources/create-source-dialog"
import { sourcesQuery } from "@/features/sources/queries"
import { SOURCE_LABEL, SourceIcon } from "@/features/sources/source-icon"

export const Route = createFileRoute("/t/$team/sources/")({ component: SourcesPage })

function SourcesPage() {
  const { team } = Route.useParams()
  const sources = useQuery(sourcesQuery(team))
  const canManage = usePermission(team, "source.manage")
  const [creating, setCreating] = useState(false)
  return (
    <>
      <PageHeader
        title="Sources Git"
        description="Accès aux dépôts privés et webhooks de déploiement automatique (push, pull requests)."
        actions={
          canManage && (
            <Button onClick={() => setCreating(true)}>
              <PlusIcon /> Nouvelle source
            </Button>
          )
        }
      />
      {sources.isPending ? (
        <Skeleton className="h-32 w-full" />
      ) : !sources.data?.length ? (
        <EmptyState
          icon={GitBranchIcon}
          title="Aucune source Git"
          description="Connectez GitHub, GitLab ou Gitea pour déployer des dépôts privés et réagir aux push et pull requests."
          action={canManage && <Button onClick={() => setCreating(true)}>Ajouter une source</Button>}
        />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {sources.data.map((s) => (
            <Link key={s.id} to="/t/$team/sources/$source" params={{ team, source: s.id }}>
              <Card className="h-full p-5 transition-colors hover:border-primary/40 hover:bg-accent/30">
                <div className="flex items-start gap-3">
                  <SourceIcon type={s.type} />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-semibold">{s.name}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {SOURCE_LABEL[s.type]} · {s.html_url.replace(/^https?:\/\//, "")}
                    </p>
                  </div>
                </div>
                <div className="mt-4">
                  {s.type === "github" && !s.installed ? (
                    <Badge variant="warning">{s.configured ? "À installer" : "En cours de création"}</Badge>
                  ) : (
                    <Badge variant="success">Connectée</Badge>
                  )}
                </div>
              </Card>
            </Link>
          ))}
        </div>
      )}
      <CreateSourceDialog team={team} open={creating} onOpenChange={setCreating} />
    </>
  )
}
