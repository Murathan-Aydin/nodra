import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { LockIcon, RefreshCwIcon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { CopyButton } from "@/components/copy-button"
import { EmptyState } from "@/components/empty-state"
import { SecretField } from "@/components/secret-field"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { get, post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { appBase } from "@/features/applications/queries"

export const Route = createFileRoute("/t/$team/applications/$app/webhooks")({ component: AppWebhooks })

interface Hooks {
  secret: string
  urls: Record<"github" | "gitlab" | "gitea" | "bitbucket" | "deploy", string>
}

const FORGES = [
  { key: "github", label: "GitHub", header: "Secret du webhook (signature X-Hub-Signature-256), type application/json." },
  { key: "gitlab", label: "GitLab", header: "« Secret token » (en-tête X-Gitlab-Token)." },
  { key: "gitea", label: "Gitea / Forgejo", header: "Secret du webhook (signature X-Gitea-Signature)." },
  { key: "bitbucket", label: "Bitbucket", header: "Secret du webhook (signature X-Hub-Signature)." },
] as const

function AppWebhooks() {
  const { team, app } = Route.useParams()
  const base = appBase(team, app)
  const allowed = usePermission(team, "resource.update", { type: "resource", id: app })
  const key = ["team", team, "app", app, "webhooks"]
  const hooks = useQuery({ queryKey: key, queryFn: () => get<Hooks>(`${base}/webhooks`), enabled: allowed })
  const [rotating, setRotating] = useState(false)
  const rotate = useApiMutation(() => post(`${base}/webhooks/rotate`), {
    success: "Nouveau secret généré : mettez à jour vos webhooks",
    invalidate: [key],
    onSuccess: () => setRotating(false),
  })
  if (!allowed) return <EmptyState icon={LockIcon} title="Accès refusé" description="La permission « Modifier la ressource » est nécessaire." />
  if (!hooks.data) return <Skeleton className="h-64" />
  const h = hooks.data
  const curl = `curl -X POST -H "Authorization: Bearer ${h.secret}" ${h.urls.deploy}`
  const apiCurl = `curl -X POST -H "Authorization: Bearer <jeton API>" "${new URL(h.urls.deploy).origin}/api/v1/teams/${team}/deploy?uuid=${app}&force=false"`
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Secret</CardTitle>
          <CardDescription>Partagé par tous les webhooks de cette application. Le renouveler invalide les webhooks existants.</CardDescription>
        </CardHeader>
        <CardContent className="flex items-center gap-2">
          <SecretField value={h.secret} className="flex-1" />
          <Button variant="outline" onClick={() => setRotating(true)}>
            <RefreshCwIcon /> Renouveler
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Déploiement depuis une CI</CardTitle>
          <CardDescription>GET ou POST ; ajouter ?force=1 pour reconstruire sans cache, ?commit=SHA pour un commit précis.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <UrlRow url={h.urls.deploy} />
          <Snippet code={curl} />
          <p className="text-sm text-muted-foreground">
            Avec un jeton API (profil → jetons), plusieurs ressources à la fois : <code className="font-mono text-xs">?uuid=a,b</code> ou{" "}
            <code className="font-mono text-xs">?tag=prod</code>.
          </p>
          <Snippet code={apiCurl} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Webhooks manuels de forge</CardTitle>
          <CardDescription>
            Inutiles avec une GitHub App ou une source GitLab/Gitea connectée. Sinon, configurez-les sur le dépôt (événements push et pull request) avec le secret
            ci-dessus.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {FORGES.map((f) => (
            <div key={f.key} className="space-y-1">
              <p className="text-sm font-medium">{f.label}</p>
              <UrlRow url={h.urls[f.key]} />
              <p className="text-xs text-muted-foreground">{f.header}</p>
            </div>
          ))}
        </CardContent>
      </Card>
      <ConfirmDialog
        open={rotating}
        onOpenChange={setRotating}
        title="Renouveler le secret ?"
        description="Les webhooks et appels CI utilisant l'ancien secret seront refusés."
        confirmLabel="Renouveler"
        destructive
        loading={rotate.isPending}
        onConfirm={() => rotate.mutate(undefined)}
      />
    </div>
  )
}

function Snippet({ code }: { code: string }) {
  return (
    <div className="flex items-start gap-2 rounded-lg bg-muted p-3">
      <code className="flex-1 font-mono text-xs break-all">{code}</code>
      <CopyButton value={code} />
    </div>
  )
}

function UrlRow({ url }: { url: string }) {
  return (
    <div className="flex items-center gap-2 rounded-md border px-3 py-1.5">
      <code className="flex-1 truncate font-mono text-xs">{url}</code>
      <CopyButton value={url} />
    </div>
  )
}
