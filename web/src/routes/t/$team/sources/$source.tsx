import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { CheckCircle2Icon, ExternalLinkIcon, LockIcon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError, del, patch, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import { reposQuery, sourceQuery } from "@/features/sources/queries"
import { SOURCE_LABEL, SourceIcon } from "@/features/sources/source-icon"

export const Route = createFileRoute("/t/$team/sources/$source")({
  validateSearch: (s: Record<string, unknown>): { installed?: string } => (s.installed === "1" || s.installed === 1 ? { installed: "1" } : {}),
  component: SourcePage,
})

function SourcePage() {
  const { team, source } = Route.useParams()
  const { installed } = Route.useSearch()
  const navigate = useNavigate()
  const src = useQuery(sourceQuery(team, source))
  const ready = !!src.data && (src.data.type !== "github" || src.data.installed)
  const repos = useQuery({ ...reposQuery(team, source), enabled: ready })
  const canManage = usePermission(team, "source.manage")
  const [form, setForm] = useState<Record<string, string>>({})
  const [deleting, setDeleting] = useState(false)
  const [filter, setFilter] = useState("")
  const save = useApiMutation(
    () => {
      const body: Record<string, unknown> = { ...form }
      if (form.installation_id !== undefined) body.installation_id = Number(form.installation_id) || 0
      return patch(teamPath(team, `/sources/${source}`), body)
    },
    {
    success: "Source mise à jour",
    invalidate: [["team", team, "source", source], ["team", team, "sources"]],
      onSuccess: () => setForm({}),
    },
  )
  const remove = useApiMutation(() => del(teamPath(team, `/sources/${source}`)), {
    success: "Source supprimée",
    invalidate: [["team", team, "sources"]],
    onSuccess: () => void navigate({ to: "/t/$team/sources", params: { team } }),
  })
  const s = src.data
  if (!s) return <Skeleton className="h-40 w-full" />
  const value = (k: string, fallback: string) => form[k] ?? fallback
  const shown = repos.data?.filter((r) => !filter || r.full_name.toLowerCase().includes(filter.toLowerCase()))
  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            <SourceIcon type={s.type} className="size-9" />
            {s.name}
          </span>
        }
        description={`${SOURCE_LABEL[s.type]} · ${s.html_url}`}
      />
      {installed && (
        <Alert variant="info" className="mb-6">
          <CheckCircle2Icon />
          <AlertDescription>GitHub App installée : vos dépôts sont accessibles.</AlertDescription>
        </Alert>
      )}
      {s.type === "github" && !s.installed && (
        <Alert variant="warning" className="mb-6">
          <AlertDescription>
            L'App n'est pas encore installée sur un compte ou une organisation.
            {s.app_slug && (
              <>
                {" "}
                <a className="font-medium underline" href={`${s.html_url.replace(/\/$/, "")}/apps/${s.app_slug}/installations/new`} target="_blank" rel="noreferrer">
                  Installer l'App
                </a>
                , puis renseignez l'Installation ID ci-dessous si GitHub ne revient pas automatiquement.
              </>
            )}
          </AlertDescription>
        </Alert>
      )}
      <div className="grid gap-6 lg:grid-cols-[1fr_minmax(0,380px)]">
        <Card>
          <CardHeader className="flex-row items-center justify-between gap-2">
            <CardTitle>Dépôts accessibles</CardTitle>
            {!!repos.data?.length && <Input className="h-8 max-w-56" placeholder="Filtrer" value={filter} onChange={(e) => setFilter(e.target.value)} />}
          </CardHeader>
          <CardContent>
            {!ready ? (
              <p className="text-sm text-muted-foreground">Disponible une fois l'App installée.</p>
            ) : repos.isPending ? (
              <Skeleton className="h-24 w-full" />
            ) : repos.error ? (
              <p className="text-sm text-destructive">{repos.error instanceof ApiError ? repos.error.message : "Liste indisponible"}</p>
            ) : !shown?.length ? (
              <p className="text-sm text-muted-foreground">Aucun dépôt.</p>
            ) : (
              <ul className="max-h-[60vh] divide-y overflow-y-auto">
                {shown.map((r) => (
                  <li key={r.full_name} className="flex items-center gap-2 py-2 text-sm">
                    {r.private && <LockIcon className="size-3.5 text-muted-foreground" />}
                    <span className="font-medium">{r.full_name}</span>
                    <Badge variant="outline" className="font-mono">
                      {r.default_branch}
                    </Badge>
                    <a href={r.html_url} target="_blank" rel="noreferrer" className="ml-auto text-muted-foreground hover:text-foreground" aria-label="Ouvrir">
                      <ExternalLinkIcon className="size-4" />
                    </a>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Réglages</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <Field label="Nom">
                <Input value={value("name", s.name)} onChange={(e) => setForm({ ...form, name: e.target.value })} disabled={!canManage} />
              </Field>
              {s.type === "github" ? (
                <>
                  <Field label="Installation ID">
                    <Input
                      value={value("installation_id", s.installation_id ? String(s.installation_id) : "")}
                      onChange={(e) => setForm({ ...form, installation_id: e.target.value })}
                      disabled={!canManage}
                    />
                  </Field>
                  <p className="text-xs text-muted-foreground">
                    App ID {s.app_id || "—"} · clé privée {s.has_private_key ? "enregistrée" : "manquante"} · secret du webhook {s.has_webhook_secret ? "enregistré" : "manquant"}
                  </p>
                </>
              ) : (
                <Field label="Nouveau jeton d'accès" hint={s.has_access_token ? "Un jeton est enregistré ; laisser vide pour le conserver." : "Aucun jeton enregistré."}>
                  <Input type="password" value={form.access_token ?? ""} onChange={(e) => setForm({ ...form, access_token: e.target.value })} disabled={!canManage} />
                </Field>
              )}
            </CardContent>
            {canManage && (
              <CardFooter className="justify-between">
                <Button variant="ghost" className="text-destructive" onClick={() => setDeleting(true)}>
                  Supprimer
                </Button>
                <Button
                  loading={save.isPending}
                  disabled={!Object.keys(form).length}
                  onClick={() => save.mutate(undefined)}
                >
                  Enregistrer
                </Button>
              </CardFooter>
            )}
          </Card>
        </div>
      </div>
      <ConfirmDialog
        open={deleting}
        onOpenChange={setDeleting}
        title={`Supprimer la source ${s.name} ?`}
        description="Impossible si des applications l'utilisent."
        confirmText={s.name}
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => remove.mutate(undefined)}
      />
    </>
  )
}
