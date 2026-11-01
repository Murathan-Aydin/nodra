import { useState } from "react"
import { CodeEditor } from "@/components/code-editor"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Input } from "@/components/ui/input"
import type { ResourceRef } from "@/features/resources/resource"
import { useResourcePatch } from "@/features/resources/use-patch"
import type { Application } from "@/lib/types"
import { SaveButton } from "./general"

/** Onglet « Compose » des applications Docker Compose : fichier brut et domaines par service. */
export function ApplicationCompose({ resource, app }: { resource: ResourceRef; app: Application }) {
  const [raw, setRaw] = useState(app.docker_compose_raw)
  const save = useResourcePatch(resource, "Compose enregistré (appliqué au prochain déploiement)")
  const services = [...new Set([...Object.keys(app.docker_compose_domains ?? {}), ...composeServices(raw)])]
  if (app.build_pack !== "dockercompose") {
    return (
      <SettingsCard title="Compose">
        <p className="text-sm text-muted-foreground">Disponible pour le build pack Docker Compose.</p>
      </SettingsCard>
    )
  }
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Fichier docker-compose"
        description={app.git_repository ? `Vide = ${app.docker_compose_location} lu dans le dépôt à chaque déploiement.` : undefined}
        actions={<SaveButton dirty={raw !== app.docker_compose_raw} pending={save.isPending} onClick={() => save.mutate({ docker_compose_raw: raw })} />}
      >
        <CodeEditor value={raw} onChange={setRaw} language="yaml" height="420px" />
      </SettingsCard>
      {services.length > 0 && <ComposeDomains key={JSON.stringify(app.docker_compose_domains)} resource={resource} app={app} services={services} />}
    </div>
  )
}

/** Noms des services d'un compose (lecture simple : clés indentées de deux espaces sous « services: »). */
function composeServices(raw: string): string[] {
  const out: string[] = []
  let inServices = false
  for (const line of raw.split("\n")) {
    if (/^services:\s*$/.test(line)) inServices = true
    else if (/^\S/.test(line)) inServices = false
    else if (inServices) {
      const m = /^ {2}([\w.-]+):\s*$/.exec(line)
      if (m) out.push(m[1])
    }
  }
  return out
}

function ComposeDomains({ resource, app, services }: { resource: ResourceRef; app: Application; services: string[] }) {
  const initial = Object.fromEntries(services.map((s) => [s, (app.docker_compose_domains?.[s] ?? []).join(", ")]))
  const [d, setD] = useState(initial)
  const save = useResourcePatch(resource, "Domaines enregistrés (appliqués au prochain déploiement)")
  const dirty = JSON.stringify(d) !== JSON.stringify(initial)
  return (
    <SettingsCard
      title="Domaines par service"
      actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ docker_compose_domains: Object.fromEntries(Object.entries(d).map(([k, v]) => [k, v.split(/[\s,]+/).filter(Boolean)])) })} />}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        {services.map((s) => (
          <Field key={s} label={s} hint="Port du conteneur possible : https://app.exemple.fr:3000">
            <Input value={d[s]} onChange={(e) => setD({ ...d, [s]: e.target.value })} className="font-mono" />
          </Field>
        ))}
      </div>
    </SettingsCard>
  )
}
