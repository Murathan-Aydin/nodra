import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import type { ResourceRef } from "@/features/resources/resource"
import { useDraft, useResourcePatch } from "@/features/resources/use-patch"
import type { Application, BuildPack } from "@/lib/types"

export const BUILD_PACKS: { value: BuildPack; label: string }[] = [
  { value: "nixpacks", label: "Nixpacks (détection automatique)" },
  { value: "dockerfile", label: "Dockerfile" },
  { value: "dockercompose", label: "Docker Compose" },
  { value: "dockerimage", label: "Image Docker" },
  { value: "static", label: "Site statique" },
]

export function SaveButton({ dirty, pending, onClick }: { dirty: boolean; pending: boolean; onClick: () => void }) {
  return (
    <Button size="sm" variant="outline" disabled={!dirty} loading={pending} onClick={onClick}>
      Enregistrer
    </Button>
  )
}

/** Onglet « Général » d'une application (configuration du fork : détails, build, réseau, commandes). */
export function ApplicationGeneral({ resource, app }: { resource: ResourceRef; app: Application }) {
  return (
    <div className="space-y-6">
      <Details resource={resource} app={app} />
      <Build resource={resource} app={app} />
      {app.build_pack !== "dockercompose" && <Network resource={resource} app={app} />}
      <Commands resource={resource} app={app} />
    </div>
  )
}

function Details({ resource, app }: { resource: ResourceRef; app: Application }) {
  const { draft: d, set, changed, dirty } = useDraft({ name: app.name, description: app.description, build_pack: app.build_pack, domains: (app.domains ?? []).join(", "), redirect: app.redirect })
  const save = useResourcePatch(resource)
  const body = { ...changed, ...(changed.domains !== undefined ? { domains: d.domains.split(/[\s,]+/).filter(Boolean) } : {}) }
  return (
    <SettingsCard title="Détails de l'application" actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(body)} />}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Nom *">
          <Input value={d.name} onChange={(e) => set("name", e.target.value)} />
        </Field>
        <Field label="Description">
          <Input value={d.description} onChange={(e) => set("description", e.target.value)} />
        </Field>
        <Field label="Build pack">
          <Select value={d.build_pack} onValueChange={(v) => set("build_pack", v as BuildPack)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {BUILD_PACKS.map((b) => (
                <SelectItem key={b.value} value={b.value}>
                  {b.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {app.build_pack !== "dockercompose" && (
          <Field label="Redirection www">
            <Select value={d.redirect} onValueChange={(v) => set("redirect", v as Application["redirect"])}>
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
        )}
        {app.build_pack !== "dockercompose" && (
          <Field label="Domaines" hint="Séparés par des virgules ; https:// ajouté si absent ; un chemin est possible (https://exemple.fr/api)." className="sm:col-span-2">
            <Input value={d.domains} onChange={(e) => set("domains", e.target.value)} className="font-mono" />
          </Field>
        )}
      </div>
    </SettingsCard>
  )
}

function Build({ resource, app }: { resource: ResourceRef; app: Application }) {
  const bp = app.build_pack
  const { draft: d, set, changed, dirty } = useDraft({
    base_directory: app.base_directory,
    publish_directory: app.publish_directory,
    install_command: app.install_command,
    build_command: app.build_command,
    start_command: app.start_command,
    dockerfile_location: app.dockerfile_location,
    dockerfile_target: app.dockerfile_target,
    dockerfile: app.dockerfile,
    docker_image: app.docker_image,
    docker_image_tag: app.docker_image_tag,
    static_image: app.static_image,
    docker_compose_location: app.docker_compose_location,
  })
  const save = useResourcePatch(resource, "Build enregistré (appliqué au prochain déploiement)")
  const text = (k: keyof typeof d, label: string, hint?: string, placeholder?: string) => (
    <Field label={label} hint={hint}>
      <Input value={d[k]} onChange={(e) => set(k, e.target.value)} className="font-mono" placeholder={placeholder} />
    </Field>
  )
  return (
    <SettingsCard title="Build" actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(changed)} />}>
      <div className="grid gap-4 sm:grid-cols-2">
        {bp === "dockerimage" ? (
          <>
            {text("docker_image", "Image", undefined, "ghcr.io/owner/image")}
            {text("docker_image_tag", "Tag", undefined, "latest")}
          </>
        ) : (
          <>
            {text("base_directory", "Dossier de base", "Racine du projet dans le dépôt.", "/")}
            {(bp === "nixpacks" || bp === "static") && text("publish_directory", "Dossier publié", "Fichiers servis (site statique).", "/dist")}
            {bp === "nixpacks" && (
              <>
                {text("install_command", "Commande d'installation", "Vide = détection automatique.")}
                {text("build_command", "Commande de build")}
                {text("start_command", "Commande de démarrage")}
              </>
            )}
            {bp === "dockerfile" && (
              <>
                {text("dockerfile_location", "Emplacement du Dockerfile", undefined, "/Dockerfile")}
                {text("dockerfile_target", "Étape cible (--target)")}
              </>
            )}
            {bp === "static" && text("static_image", "Image du serveur web", undefined, "nginx:alpine")}
            {bp === "dockercompose" && text("docker_compose_location", "Emplacement du compose", undefined, "/docker-compose.yaml")}
          </>
        )}
        {bp === "dockerfile" && (
          <Field label="Dockerfile en ligne" hint="Remplace le fichier du dépôt s'il est rempli." className="sm:col-span-2">
            <Textarea value={d.dockerfile} onChange={(e) => set("dockerfile", e.target.value)} rows={8} className="font-mono" spellCheck={false} />
          </Field>
        )}
      </div>
    </SettingsCard>
  )
}

function Network({ resource, app }: { resource: ResourceRef; app: Application }) {
  const { draft: d, set, changed, dirty } = useDraft({ ports_exposes: app.ports_exposes, ports_mappings: app.ports_mappings })
  const save = useResourcePatch(resource, "Réseau enregistré (appliqué au prochain déploiement)")
  return (
    <SettingsCard title="Réseau" actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(changed)} />}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Ports exposés" hint="Ports du conteneur, ex. « 3000 » ou « 3000,3001 ». Le premier reçoit les domaines.">
          <Input value={d.ports_exposes} onChange={(e) => set("ports_exposes", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Ports publiés sur le serveur" hint="Ex. « 8080:80 ». Empêche la bascule sans coupure.">
          <Input value={d.ports_mappings} onChange={(e) => set("ports_mappings", e.target.value)} className="font-mono" />
        </Field>
      </div>
    </SettingsCard>
  )
}

function Commands({ resource, app }: { resource: ResourceRef; app: Application }) {
  const { draft: d, set, changed, dirty } = useDraft({
    pre_deployment_command: app.pre_deployment_command,
    pre_deployment_container: app.pre_deployment_container,
    post_deployment_command: app.post_deployment_command,
    post_deployment_container: app.post_deployment_container,
  })
  const save = useResourcePatch(resource)
  const compose = app.build_pack === "dockercompose"
  return (
    <SettingsCard title="Commandes de déploiement" description="Exécutées dans le conteneur : avant (conteneur en cours) et après le démarrage de la nouvelle version." actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(changed)} />}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Avant le déploiement">
          <Input value={d.pre_deployment_command} onChange={(e) => set("pre_deployment_command", e.target.value)} className="font-mono" placeholder="php artisan migrate" />
        </Field>
        {compose && (
          <Field label="Conteneur (avant)">
            <Input value={d.pre_deployment_container} onChange={(e) => set("pre_deployment_container", e.target.value)} className="font-mono" />
          </Field>
        )}
        <Field label="Après le déploiement">
          <Input value={d.post_deployment_command} onChange={(e) => set("post_deployment_command", e.target.value)} className="font-mono" />
        </Field>
        {compose && (
          <Field label="Conteneur (après)">
            <Input value={d.post_deployment_container} onChange={(e) => set("post_deployment_container", e.target.value)} className="font-mono" />
          </Field>
        )}
      </div>
    </SettingsCard>
  )
}
