import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import { CodeEditor } from "@/components/code-editor"
import { Field } from "@/components/field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { patch } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Application, Tag } from "@/lib/types"
import { appBase, appQuery } from "@/features/applications/queries"
import { ResourceActions } from "@/features/resources/resource-actions"
import { TagPicker } from "@/features/resources/tag-picker"

export const Route = createFileRoute("/t/$team/applications/$app/advanced")({ component: AppAdvanced })

type Settings = Record<string, unknown>
type Toggle = [key: string, label: string, hint?: string]

const BUILD: Toggle[] = [
  ["auto_deploy", "Déploiement automatique au push"],
  ["disable_build_cache", "Désactiver le cache de build"],
  ["inject_build_args", "Variables de build passées en --build-arg"],
  ["use_build_secrets", "Variables de build en secrets Docker", "Évite qu'elles restent dans les couches de l'image."],
  ["include_source_commit_in_build", "Commit source disponible pendant le build (SOURCE_COMMIT)"],
  ["include_default_variables", "Variables NODRA_* injectées"],
  ["git_submodules", "Sous-modules Git"],
  ["git_lfs", "Git LFS"],
  ["git_shallow_clone", "Clone superficiel (--depth 1)"],
  ["preserve_repository", "Conserver le dépôt cloné sur le serveur"],
  ["debug", "Logs de build détaillés"],
]
const NETWORK: Toggle[] = [
  ["force_https", "Forcer HTTPS"],
  ["permanent_redirect", "Redirections permanentes (301/308)"],
  ["gzip", "Compression gzip"],
  ["strip_prefix", "Retirer le préfixe de chemin avant le conteneur"],
  ["is_spa", "Application monopage (repli sur index.html)"],
  ["consistent_container_name", "Nom de conteneur stable (sans suffixe de déploiement)"],
  ["connect_to_docker_network", "Connecter au réseau Docker de la destination (compose)"],
  ["container_label_escape", "Échapper les $ des labels"],
  ["container_label_readonly", "Labels générés en lecture seule", "Désactiver pour éditer les labels Traefik/Caddy ci-dessous."],
]
const LOGS: Toggle[] = [
  ["log_drain_enabled", "Envoyer les logs au log drain du serveur"],
  ["include_timestamps", "Horodatage dans les logs des conteneurs"],
  ["redact_env_values_in_logs", "Masquer les valeurs des variables dans les logs de déploiement"],
]

function AppAdvanced() {
  const { team, app } = Route.useParams()
  const { data } = useQuery(appQuery(team, app))
  if (!data) return null
  return <AdvancedForm key={data.application.updated_at} team={team} app={data.application} tags={data.tags} />
}

function AdvancedForm({ team, app, tags }: { team: string; app: Application; tags: Tag[] }) {
  const canUpdate = usePermission(team, "resource.update", { type: "resource", id: app.id })
  const base = appBase(team, app.id)
  const [s, setS] = useState<Settings>({ ...app.settings })
  const [f, setF] = useState({
    custom_labels: app.custom_labels,
    custom_docker_run_options: app.custom_docker_run_options,
    pre_deployment_command: app.pre_deployment_command,
    pre_deployment_container: app.pre_deployment_container,
    post_deployment_command: app.post_deployment_command,
    post_deployment_container: app.post_deployment_container,
  })
  const [password, setPassword] = useState("")
  const save = useApiMutation(
    () => {
      const settings = { ...s }
      delete settings.basic_auth_password_hash
      return patch(base, { ...f, settings, ...(password ? { basic_auth_password: password } : {}) })
    },
    { success: "Réglages enregistrés (appliqués au prochain déploiement)", invalidate: [["team", team, "app", app.id]] },
  )
  const disabled = !canUpdate
  const bool = (k: string) => !!s[k]
  const str = (k: string) => String(s[k] ?? "")
  const num = (k: string) => Number(s[k] ?? 0)
  const toggles = (list: Toggle[]) =>
    list
      .filter(([k]) => (k === "is_spa" ? app.build_pack === "static" : k === "connect_to_docker_network" ? app.build_pack === "dockercompose" : true))
      .map(([k, label, hint]) => (
        <Label key={k} className="flex items-start justify-between gap-4 font-normal">
          <span>
            {label}
            {hint && <span className="block text-xs text-muted-foreground">{hint}</span>}
          </span>
          <Switch checked={bool(k)} onCheckedChange={(v) => setS({ ...s, [k]: v })} disabled={disabled} />
        </Label>
      ))

  return (
    <div className="space-y-6">
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate(undefined)
        }}
      >
        <fieldset disabled={disabled} className="grid gap-6 lg:grid-cols-2">
          <Section title="Build et déploiement">{toggles(BUILD)}</Section>
          <div className="space-y-6">
            <Section title="Réseau et proxy">{toggles(NETWORK)}</Section>
            <Section title="Logs">{toggles(LOGS)}</Section>
          </div>

          <Section title="Conservation et arrêt">
            <div className="grid gap-4 sm:grid-cols-3">
              <Field label="Images conservées" hint="Pour le retour arrière (0-50).">
                <Input type="number" min={0} max={50} value={num("docker_images_to_keep")} onChange={(e) => setS({ ...s, docker_images_to_keep: Number(e.target.value) })} />
              </Field>
              <Field label="Arrêt gracieux (s)">
                <Input type="number" min={0} max={3600} value={num("stop_grace_period")} onChange={(e) => setS({ ...s, stop_grace_period: Number(e.target.value) })} />
              </Field>
              <Field label="Drainage (s)" hint="Ancien conteneur gardé après bascule.">
                <Input
                  type="number"
                  min={0}
                  value={num("rolling_update_drain_seconds")}
                  onChange={(e) => setS({ ...s, rolling_update_drain_seconds: Number(e.target.value) })}
                />
              </Field>
            </div>
            <Field label="Nom interne personnalisé" hint="Alias réseau joignable par les autres conteneurs.">
              <Input className="font-mono" value={str("custom_internal_name")} onChange={(e) => setS({ ...s, custom_internal_name: e.target.value })} />
            </Field>
          </Section>

          <Section
            title="Authentification HTTP"
            description="Protège les domaines par un identifiant et un mot de passe (basic auth au niveau du proxy)."
            badge={app.basic_auth_password_set ? <Badge variant="success">Mot de passe défini</Badge> : undefined}
          >
            <Label className="flex items-center justify-between font-normal">
              Activer
              <Switch checked={bool("basic_auth_enabled")} onCheckedChange={(v) => setS({ ...s, basic_auth_enabled: v })} disabled={disabled} />
            </Label>
            {bool("basic_auth_enabled") && (
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Identifiant">
                  <Input value={str("basic_auth_username")} onChange={(e) => setS({ ...s, basic_auth_username: e.target.value })} autoComplete="off" />
                </Field>
                <Field label="Mot de passe" hint={app.basic_auth_password_set ? "Laisser vide pour le conserver." : undefined}>
                  <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
                </Field>
              </div>
            )}
          </Section>

          <Section title="Commandes de déploiement" description="Exécutées dans un conteneur (le principal par défaut, ou un service compose).">
            <div className="grid gap-4 sm:grid-cols-[1fr_10rem]">
              <Field label="Avant la bascule">
                <Input className="font-mono" value={f.pre_deployment_command} onChange={(e) => setF({ ...f, pre_deployment_command: e.target.value })} placeholder="php artisan migrate --force" />
              </Field>
              <Field label="Conteneur">
                <Input className="font-mono" value={f.pre_deployment_container} onChange={(e) => setF({ ...f, pre_deployment_container: e.target.value })} />
              </Field>
              <Field label="Après la bascule">
                <Input className="font-mono" value={f.post_deployment_command} onChange={(e) => setF({ ...f, post_deployment_command: e.target.value })} />
              </Field>
              <Field label="Conteneur">
                <Input className="font-mono" value={f.post_deployment_container} onChange={(e) => setF({ ...f, post_deployment_container: e.target.value })} />
              </Field>
            </div>
          </Section>

          <Section title="GPU">
            <Label className="flex items-center justify-between font-normal">
              Réserver des GPU
              <Switch checked={bool("gpu_enabled")} onCheckedChange={(v) => setS({ ...s, gpu_enabled: v })} disabled={disabled} />
            </Label>
            {bool("gpu_enabled") && (
              <div className="grid gap-4 sm:grid-cols-3">
                <Field label="Pilote">
                  <Input className="font-mono" value={str("gpu_driver")} onChange={(e) => setS({ ...s, gpu_driver: e.target.value })} placeholder="nvidia" />
                </Field>
                <Field label="Nombre">
                  <Input className="font-mono" value={str("gpu_count")} onChange={(e) => setS({ ...s, gpu_count: e.target.value })} placeholder="all" />
                </Field>
                <Field label="Identifiants">
                  <Input className="font-mono" value={str("gpu_device_ids")} onChange={(e) => setS({ ...s, gpu_device_ids: e.target.value })} />
                </Field>
              </div>
            )}
          </Section>

          <Section title="Options docker run" description="Traduites en options compose : --cap-add, --device, --ulimit, --init, --privileged…" className="lg:col-span-2">
            <Input
              className="font-mono"
              value={f.custom_docker_run_options}
              onChange={(e) => setF({ ...f, custom_docker_run_options: e.target.value })}
              placeholder="--cap-add SYS_ADMIN --init"
            />
          </Section>

          <Section title="Labels personnalisés" description="Un label par ligne ; remplacent les labels générés pour le proxy." className="lg:col-span-2">
            <CodeEditor value={f.custom_labels} onChange={(v) => setF({ ...f, custom_labels: v })} readOnly={disabled || bool("container_label_readonly")} minHeight="160px" />
          </Section>
        </fieldset>
        {canUpdate && (
          <div className="sticky bottom-4 flex justify-end">
            <Button type="submit" size="lg" loading={save.isPending} className="shadow-lg">
              Enregistrer
            </Button>
          </div>
        )}
      </form>

      <Card>
        <CardHeader>
          <CardTitle>Tags</CardTitle>
        </CardHeader>
        <CardContent>
          <TagPicker team={team} base={base} current={tags} invalidate={[["team", team, "app", app.id]]} disabled={disabled} />
        </CardContent>
      </Card>
      <ResourceActions team={team} kind="application" id={app.id} name={app.name} base={base} environmentId={app.environment_id} destinationId={app.destination_id} />
    </div>
  )
}

function Section({ title, description, badge, className, children }: { title: string; description?: string; badge?: ReactNode; className?: string; children: ReactNode }) {
  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {title}
          {badge}
        </CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent className="space-y-4">{children}</CardContent>
    </Card>
  )
}
