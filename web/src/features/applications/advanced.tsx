import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { SwitchRow } from "@/features/resources/operations"
import type { ResourceRef } from "@/features/resources/resource"
import { useDraft, useResourcePatch } from "@/features/resources/use-patch"
import type { Application } from "@/lib/types"
import { SaveButton } from "./general"

const TOGGLES: { group: string; items: [string, string, string?][] }[] = [
  {
    group: "Déploiement",
    items: [
      ["auto_deploy", "Déploiement automatique", "À chaque push sur la branche suivie (webhook)."],
      ["preview_deployments", "Previews des pull requests", "Une instance par PR ouverte."],
      ["public_pr_deployments", "Previews des PR de forks", "Attention : code non relu."],
      ["disable_build_cache", "Désactiver le cache de build"],
      ["include_source_commit_in_build", "Inclure le commit dans le build", "Variable SOURCE_COMMIT (invalide le cache)."],
      ["inject_build_args", "Variables en arguments de build"],
      ["use_build_secrets", "Secrets de build Docker", "Au lieu d'arguments visibles dans l'image."],
      ["include_default_variables", "Variables Nodra par défaut", "NODRA_FQDN, NODRA_URL, NODRA_BRANCH…"],
      ["redact_env_values_in_logs", "Masquer les valeurs des variables dans les logs"],
      ["debug", "Logs de débogage"],
    ],
  },
  {
    group: "Git",
    items: [
      ["git_submodules", "Sous-modules"],
      ["git_lfs", "Git LFS"],
      ["git_shallow_clone", "Clone superficiel"],
    ],
  },
  {
    group: "Proxy",
    items: [
      ["force_https", "Forcer HTTPS"],
      ["gzip", "Compression gzip"],
      ["strip_prefix", "Retirer le préfixe du chemin"],
      ["is_spa", "Application monopage (SPA)", "Sites statiques : renvoie index.html pour toute route."],
      ["permanent_redirect", "Redirections permanentes (308)"],
    ],
  },
  {
    group: "Conteneur",
    items: [
      ["consistent_container_name", "Nom de conteneur fixe", "Empêche la bascule sans coupure."],
      ["connect_to_docker_network", "Connecter au réseau Docker de la destination (compose)"],
      ["container_label_escape", "Échapper les labels du conteneur"],
      ["gpu_enabled", "GPU"],
    ],
  },
]

/** Onglet « Avancé » (Advanced du fork) : options de déploiement, Git, proxy et conteneur. */
export function ApplicationAdvanced({ resource, app }: { resource: ResourceRef; app: Application }) {
  return (
    <div className="space-y-6">
      {TOGGLES.map((g) => (
        <Toggles key={g.group} resource={resource} app={app} group={g} />
      ))}
      <Container resource={resource} app={app} />
      <BasicAuth resource={resource} app={app} />
    </div>
  )
}

function Toggles({ resource, app, group }: { resource: ResourceRef; app: Application; group: (typeof TOGGLES)[number] }) {
  const initial = Object.fromEntries(group.items.map(([k]) => [k, Boolean(app.settings[k])]))
  const { draft: d, set, changed, dirty } = useDraft(initial)
  const save = useResourcePatch(resource)
  return (
    <SettingsCard title={group.group} actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ settings: changed })} />}>
      <div className="grid gap-x-8 sm:grid-cols-2">
        {group.items.map(([k, label, hint]) => (
          <SwitchRow key={k} label={label} hint={hint} checked={d[k]} onChange={(v) => set(k, v)} />
        ))}
      </div>
    </SettingsCard>
  )
}

function Container({ resource, app }: { resource: ResourceRef; app: Application }) {
  const { draft: d, set, changed, dirty } = useDraft({
    custom_docker_run_options: app.custom_docker_run_options,
    custom_labels: app.custom_labels,
    docker_images_to_keep: String(app.settings.docker_images_to_keep ?? 2),
    stop_grace_period: String(app.settings.stop_grace_period ?? 30),
    custom_internal_name: String(app.settings.custom_internal_name ?? ""),
  })
  const save = useResourcePatch(resource)
  const body = () => {
    const { docker_images_to_keep, stop_grace_period, custom_internal_name, ...rest } = changed
    const settings: Record<string, unknown> = {}
    if (docker_images_to_keep !== undefined) settings.docker_images_to_keep = Number(docker_images_to_keep)
    if (stop_grace_period !== undefined) settings.stop_grace_period = Number(stop_grace_period)
    if (custom_internal_name !== undefined) settings.custom_internal_name = custom_internal_name
    return { ...rest, ...(Object.keys(settings).length ? { settings } : {}) }
  }
  return (
    <SettingsCard title="Conteneur (options)" actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(body())} />}>
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Images gardées" hint="Pour revenir en arrière.">
          <Input value={d.docker_images_to_keep} onChange={(e) => set("docker_images_to_keep", e.target.value.replace(/\D/g, ""))} />
        </Field>
        <Field label="Délai d'arrêt (s)">
          <Input value={d.stop_grace_period} onChange={(e) => set("stop_grace_period", e.target.value.replace(/\D/g, ""))} />
        </Field>
        <Field label="Nom interne" hint="Alias réseau du conteneur.">
          <Input value={d.custom_internal_name} onChange={(e) => set("custom_internal_name", e.target.value)} className="font-mono" />
        </Field>
        <Field label="Options docker run" className="sm:col-span-3">
          <Input value={d.custom_docker_run_options} onChange={(e) => set("custom_docker_run_options", e.target.value)} className="font-mono" placeholder="--cap-add=SYS_ADMIN --shm-size=256m" />
        </Field>
        <Field label="Labels personnalisés" hint="Un label par ligne ; remplacent les labels générés de même nom." className="sm:col-span-3">
          <Textarea value={d.custom_labels} onChange={(e) => set("custom_labels", e.target.value)} rows={6} className="font-mono" spellCheck={false} />
        </Field>
      </div>
    </SettingsCard>
  )
}

function BasicAuth({ resource, app }: { resource: ResourceRef; app: Application }) {
  const { draft: d, set, dirty } = useDraft({ enabled: Boolean(app.settings.basic_auth_enabled), username: String(app.settings.basic_auth_username ?? ""), password: "" })
  const save = useResourcePatch(resource, "Authentification enregistrée")
  return (
    <SettingsCard
      title="Authentification HTTP basique"
      description={app.basic_auth_password_set ? "Un mot de passe est défini (stocké en bcrypt)." : "Protège l'application par un identifiant et un mot de passe."}
      actions={
        <SaveButton
          dirty={dirty}
          pending={save.isPending}
          onClick={() => save.mutate({ settings: { basic_auth_enabled: d.enabled, basic_auth_username: d.username }, ...(d.password ? { basic_auth_password: d.password } : {}) })}
        />
      }
    >
      <SwitchRow label="Activée" checked={d.enabled} onChange={(v) => set("enabled", v)} />
      {d.enabled && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Identifiant">
            <Input value={d.username} onChange={(e) => set("username", e.target.value)} />
          </Field>
          <Field label="Mot de passe" hint={app.basic_auth_password_set ? "Vide = inchangé." : undefined}>
            <Input type="password" value={d.password} onChange={(e) => set("password", e.target.value)} />
          </Field>
        </div>
      )}
    </SettingsCard>
  )
}
