import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import { CheckCircle2Icon, SearchCheckIcon, XCircleIcon } from "lucide-react"
import { CodeEditor } from "@/components/code-editor"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { ApiError, patch, post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Application, HealthCheck, Limits } from "@/lib/types"
import { appBase, appQuery, BUILD_PACKS } from "@/features/applications/queries"

export const Route = createFileRoute("/t/$team/applications/$app/")({ component: AppGeneral })

function AppGeneral() {
  const { team, app } = Route.useParams()
  const { data } = useQuery(appQuery(team, app))
  if (!data) return null
  return <AppForm key={data.application.updated_at} team={team} app={data.application} serverId={data.server_id} />
}

type Form = Application & { domains_text: string; compose_domains_text: string }

// Champs envoyés au PATCH (l'API refuse les champs inconnus ; settings est édité dans « Avancé »).
const EDITABLE = [
  "name",
  "description",
  "git_repository",
  "git_branch",
  "git_commit_sha",
  "build_pack",
  "docker_image",
  "docker_image_tag",
  "static_image",
  "base_directory",
  "publish_directory",
  "dockerfile",
  "dockerfile_location",
  "dockerfile_target",
  "docker_compose_location",
  "docker_compose_raw",
  "install_command",
  "build_command",
  "start_command",
  "watch_paths",
  "ports_exposes",
  "ports_mappings",
  "redirect",
  "preview_url_template",
  "health_check",
  "limits",
] as const satisfies (keyof Application)[]

function AppForm({ team, app, serverId }: { team: string; app: Application; serverId: string }) {
  const canUpdate = usePermission(team, "resource.update", { type: "resource", id: app.id })
  const [f, setF] = useState<Form>({
    ...app,
    domains_text: app.domains.join("\n"),
    compose_domains_text: Object.entries(app.docker_compose_domains ?? {})
      .map(([svc, ds]) => `${svc}=${ds.join(",")}`)
      .join("\n"),
  })
  const [conflict, setConflict] = useState("")
  const save = useApiMutation(
    async (force: boolean) => {
      const body: Record<string, unknown> = Object.fromEntries(EDITABLE.map((k) => [k, f[k]]))
      body.domains = f.domains_text.split(/[\s,]+/).filter(Boolean)
      body.docker_compose_domains = Object.fromEntries(
        f.compose_domains_text
          .split("\n")
          .map((l) => l.trim())
          .filter((l) => l.includes("="))
          .map((l) => {
            const [svc, ds] = l.split("=")
            return [svc.trim(), ds.split(/[\s,]+/).filter(Boolean)]
          }),
      )
      if (force) body.force_domain_override = true
      try {
        return await patch<Application>(appBase(team, app.id), body)
      } catch (e) {
        // Domaine déjà utilisé par une autre ressource : proposer de forcer.
        if (e instanceof ApiError && e.status === 409) {
          setConflict(e.message)
          return null
        }
        throw e
      }
    },
    {
      success: (r) => (r ? "Configuration enregistrée (appliquée au prochain déploiement)" : undefined),
      invalidate: [["team", team, "app", app.id]],
      onSuccess: (r) => r && setConflict(""),
    },
  )
  const set = <K extends keyof Form>(k: K) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setF({ ...f, [k]: e.target.value })
  const hc = <K extends keyof HealthCheck>(k: K, v: HealthCheck[K]) => setF({ ...f, health_check: { ...f.health_check, [k]: v } })
  const lim = <K extends keyof Limits>(k: K, v: Limits[K]) => setF({ ...f, limits: { ...f.limits, [k]: v } })
  const git = !!f.git_repository && f.build_pack !== "dockerimage"
  const disabled = !canUpdate

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(false)
      }}
    >
      <fieldset disabled={disabled} className="space-y-6">
        <Section title="Général">
          <Field label="Nom">
            <Input value={f.name} onChange={set("name")} />
          </Field>
          <Field label="Description">
            <Input value={f.description} onChange={set("description")} />
          </Field>
          <Field label="Build pack">
            <Select value={f.build_pack} onValueChange={(v) => setF({ ...f, build_pack: v as Application["build_pack"] })} disabled={disabled}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(BUILD_PACKS).map(([k, v]) => (
                  <SelectItem key={k} value={k}>
                    {v.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        </Section>

        {f.build_pack === "dockerimage" ? (
          <Section title="Image">
            <Field label="Image">
              <Input className="font-mono" value={f.docker_image} onChange={set("docker_image")} />
            </Field>
            <Field label="Tag">
              <Input className="font-mono" value={f.docker_image_tag} onChange={set("docker_image_tag")} />
            </Field>
          </Section>
        ) : (
          (git || f.git_repository) && (
            <Section title="Dépôt">
              <Field label="Dépôt">
                <Input className="font-mono" value={f.git_repository} onChange={set("git_repository")} />
              </Field>
              <Field label="Branche">
                <Input className="font-mono" value={f.git_branch} onChange={set("git_branch")} />
              </Field>
              <Field label="Commit" hint="HEAD ou un SHA précis.">
                <Input className="font-mono" value={f.git_commit_sha} onChange={set("git_commit_sha")} />
              </Field>
              <Field label="Dossier de base">
                <Input className="font-mono" value={f.base_directory} onChange={set("base_directory")} />
              </Field>
              <Field label="Chemins surveillés" hint="Un motif par ligne : le push ne redéploie que s'ils changent." className="md:col-span-2">
                <Textarea rows={2} className="font-mono text-xs" value={f.watch_paths} onChange={set("watch_paths")} />
              </Field>
            </Section>
          )
        )}

        {f.build_pack === "nixpacks" && (
          <Section title="Build (Nixpacks)" description="Laisser vide pour la détection automatique.">
            <Field label="Installation">
              <Input className="font-mono" value={f.install_command} onChange={set("install_command")} placeholder="npm ci" />
            </Field>
            <Field label="Build">
              <Input className="font-mono" value={f.build_command} onChange={set("build_command")} placeholder="npm run build" />
            </Field>
            <Field label="Démarrage" className="md:col-span-2">
              <Input className="font-mono" value={f.start_command} onChange={set("start_command")} placeholder="npm start" />
            </Field>
          </Section>
        )}
        {f.build_pack === "static" && (
          <Section title="Site statique">
            <Field label="Dossier publié" hint="Résultat du build (ex. dist).">
              <Input className="font-mono" value={f.publish_directory} onChange={set("publish_directory")} />
            </Field>
            <Field label="Image du serveur web">
              <Input className="font-mono" value={f.static_image} onChange={set("static_image")} placeholder="nginx:alpine" />
            </Field>
            <Field label="Build">
              <Input className="font-mono" value={f.build_command} onChange={set("build_command")} />
            </Field>
          </Section>
        )}
        {f.build_pack === "dockerfile" && (
          <Section title="Dockerfile">
            <Field label="Emplacement dans le dépôt">
              <Input className="font-mono" value={f.dockerfile_location} onChange={set("dockerfile_location")} />
            </Field>
            <Field label="Étape cible (target)">
              <Input className="font-mono" value={f.dockerfile_target} onChange={set("dockerfile_target")} />
            </Field>
            <Field label="Dockerfile écrit ici (prioritaire)" className="md:col-span-2">
              <CodeEditor value={f.dockerfile} onChange={(v) => setF({ ...f, dockerfile: v })} readOnly={disabled} minHeight="160px" />
            </Field>
          </Section>
        )}
        {f.build_pack === "dockercompose" && (
          <Section title="Docker Compose" description="Emplacement dans le dépôt, ou contenu écrit ici (prioritaire).">
            <Field label="Emplacement" className="md:col-span-2">
              <Input className="font-mono" value={f.docker_compose_location} onChange={set("docker_compose_location")} />
            </Field>
            <Field label="docker-compose.yml" className="md:col-span-2">
              <CodeEditor language="yaml" value={f.docker_compose_raw} onChange={(v) => setF({ ...f, docker_compose_raw: v })} readOnly={disabled} minHeight="220px" />
            </Field>
            <Field label="Domaines par service" hint="Une ligne par service : web=https://app.example.com,https://www.example.com" className="md:col-span-2">
              <Textarea rows={3} className="font-mono text-xs" value={f.compose_domains_text} onChange={set("compose_domains_text")} />
            </Field>
          </Section>
        )}

        {f.build_pack !== "dockercompose" && (
          <Section title="Réseau et domaines">
            <Field label="Domaines" hint="Un par ligne ; https:// est ajouté si absent ; un port précise celui du conteneur." className="md:col-span-2">
              <Textarea rows={3} className="font-mono text-xs" value={f.domains_text} onChange={set("domains_text")} />
            </Field>
            <div className="md:col-span-2">
              <DnsCheck team={team} serverId={serverId} domains={f.domains_text.split(/[\s,]+/).filter(Boolean)} />
            </div>
            <Field label="Ports exposés" hint="ex. 3000">
              <Input className="font-mono" value={f.ports_exposes} onChange={set("ports_exposes")} />
            </Field>
            <Field label="Ports publiés sur l'hôte" hint="ex. 8080:80 (sans proxy)">
              <Input className="font-mono" value={f.ports_mappings} onChange={set("ports_mappings")} />
            </Field>
            <Field label="Redirection www">
              <Select value={f.redirect} onValueChange={(v) => setF({ ...f, redirect: v as Application["redirect"] })} disabled={disabled}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="both">Aucune (www et sans www)</SelectItem>
                  <SelectItem value="www">Vers www</SelectItem>
                  <SelectItem value="non-www">Vers sans www</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Modèle d'URL des previews" hint="{{pr_id}}, {{domain}}, {{random}}">
              <Input className="font-mono" value={f.preview_url_template} onChange={set("preview_url_template")} />
            </Field>
          </Section>
        )}

        <Section title="Vérification de santé">
          <Label className="flex items-center justify-between font-normal md:col-span-2">
            <span>Activer la vérification de santé (bascule sans coupure)</span>
            <Switch checked={f.health_check.enabled} onCheckedChange={(v) => hc("enabled", v)} disabled={disabled} />
          </Label>
          {f.health_check.enabled && (
            <>
              <Field label="Type">
                <Select value={f.health_check.type} onValueChange={(v) => hc("type", v as HealthCheck["type"])} disabled={disabled}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="http">HTTP</SelectItem>
                    <SelectItem value="cmd">Commande</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              {f.health_check.type === "http" ? (
                <>
                  <Field label="Chemin">
                    <Input className="font-mono" value={f.health_check.path} onChange={(e) => hc("path", e.target.value)} />
                  </Field>
                  <Field label="Port">
                    <Input className="font-mono" value={f.health_check.port} onChange={(e) => hc("port", e.target.value)} />
                  </Field>
                  <Field label="Code attendu">
                    <Input type="number" value={f.health_check.return_code} onChange={(e) => hc("return_code", Number(e.target.value))} />
                  </Field>
                </>
              ) : (
                <Field label="Commande">
                  <Input className="font-mono" value={f.health_check.command} onChange={(e) => hc("command", e.target.value)} />
                </Field>
              )}
              <Field label="Intervalle / délai / essais (s)">
                <div className="flex gap-2">
                  <Input type="number" value={f.health_check.interval} onChange={(e) => hc("interval", Number(e.target.value))} />
                  <Input type="number" value={f.health_check.timeout} onChange={(e) => hc("timeout", Number(e.target.value))} />
                  <Input type="number" value={f.health_check.retries} onChange={(e) => hc("retries", Number(e.target.value))} />
                </div>
              </Field>
            </>
          )}
        </Section>

        <Section title="Limites de ressources" description="0 = illimité.">
          <Field label="Mémoire" hint="ex. 512m">
            <Input className="font-mono" value={f.limits.memory} onChange={(e) => lim("memory", e.target.value)} />
          </Field>
          <Field label="CPU" hint="ex. 1.5">
            <Input className="font-mono" value={f.limits.cpus} onChange={(e) => lim("cpus", e.target.value)} />
          </Field>
        </Section>
      </fieldset>
      <ConfirmDialog
        open={!!conflict}
        onOpenChange={(o) => !o && setConflict("")}
        title="Domaine déjà utilisé"
        description={`${conflict}. Enregistrer quand même ?`}
        confirmLabel="Forcer"
        destructive
        loading={save.isPending}
        onConfirm={() => save.mutate(true)}
      />
      {canUpdate && (
        <div className="sticky bottom-4 flex justify-end">
          <Button type="submit" size="lg" loading={save.isPending} className="shadow-lg">
            Enregistrer
          </Button>
        </div>
      )}
    </form>
  )
}

function Section({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent className="grid gap-4 md:grid-cols-2">{children}</CardContent>
    </Card>
  )
}

interface DnsResult {
  domain: string
  resolved: string[]
  expected: string[]
  ok: boolean
  message: string
  skipped?: boolean
}

function DnsCheck({ team, serverId, domains }: { team: string; serverId: string; domains: string[] }) {
  const [results, setResults] = useState<DnsResult[]>([])
  const check = useApiMutation(
    () => Promise.all(domains.filter((d) => !d.includes("sslip.io")).map((domain) => post<DnsResult>(teamPath(team, "/dns-check"), { domain, server_id: serverId }))),
    { onSuccess: setResults },
  )
  if (!domains.length) return null
  return (
    <div className="space-y-2">
      <Button type="button" size="sm" variant="outline" loading={check.isPending} onClick={() => check.mutate(undefined)}>
        <SearchCheckIcon /> Vérifier le DNS
      </Button>
      {results.map((r) => (
        <p key={r.domain} className="flex items-start gap-2 text-xs">
          {r.ok ? <CheckCircle2Icon className="size-4 shrink-0 text-success" /> : <XCircleIcon className="size-4 shrink-0 text-destructive" />}
          <span>
            <strong>{r.domain}</strong> : {r.message}
          </span>
        </p>
      ))}
    </div>
  )
}
