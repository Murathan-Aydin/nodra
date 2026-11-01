import { useState } from "react"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { SwitchRow } from "@/features/resources/operations"
import { LimitsCard } from "@/features/resources/limits"
import type { ResourceRef } from "@/features/resources/resource"
import { useResourcePatch } from "@/features/resources/use-patch"
import type { Database } from "@/lib/types"

export const DB_LABEL: Record<Database["type"], string> = {
  postgresql: "PostgreSQL",
  mysql: "MySQL",
  mariadb: "MariaDB",
  mongodb: "MongoDB",
  redis: "Redis",
  keydb: "KeyDB",
  dragonfly: "Dragonfly",
  clickhouse: "ClickHouse",
}

const KEY_VALUE = new Set(["redis", "keydb", "dragonfly"])
const CONFIG_FILE: Partial<Record<Database["type"], string>> = { postgresql: "postgresql.conf", mysql: "my.cnf", mariadb: "my.cnf", mongodb: "mongod.conf", redis: "redis.conf", keydb: "keydb.conf" }
const SSL_MODES: Partial<Record<Database["type"], string[]>> = {
  postgresql: ["require", "verify-ca", "verify-full", "prefer", "allow"],
  mongodb: ["requireTLS", "preferTLS", "allowTLS"],
}
const INIT_SCRIPTS = new Set(["postgresql", "mysql", "mariadb", "mongodb"])

/** Onglet « Général » d'une base (configuration du fork : détails, accès, conteneur, limites…). */
export function DatabaseGeneral({ resource, db }: { resource: ResourceRef; db: Database }) {
  return (
    <div className="space-y-6">
      <Details resource={resource} db={db} />
      <Credentials resource={resource} db={db} />
      <Access resource={resource} db={db} />
      {(db.type in SSL_MODES || ["mysql", "mariadb", "redis", "keydb"].includes(db.type)) && <Tls resource={resource} db={db} />}
      {CONFIG_FILE[db.type] && <CustomConfig resource={resource} db={db} />}
      {INIT_SCRIPTS.has(db.type) && <InitScripts resource={resource} db={db} />}
      <Container resource={resource} db={db} />
      <LimitsCard resource={resource} limits={db.limits} />
    </div>
  )
}

function SaveButton({ dirty, pending, onClick }: { dirty: boolean; pending: boolean; onClick: () => void }) {
  return (
    <Button size="sm" variant="outline" disabled={!dirty} loading={pending} onClick={onClick}>
      Enregistrer
    </Button>
  )
}

function Details({ resource, db }: { resource: ResourceRef; db: Database }) {
  const [d, setD] = useState({ name: db.name, description: db.description, image: db.image })
  const save = useResourcePatch(resource)
  const dirty = d.name !== db.name || d.description !== db.description || d.image !== db.image
  return (
    <SettingsCard title="Détails de la base" description={DB_LABEL[db.type]} actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate(d)} />}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Nom *">
          <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
        </Field>
        <Field label="Description">
          <Input value={d.description} onChange={(e) => setD({ ...d, description: e.target.value })} />
        </Field>
        <Field label="Image" hint="Appliquée au prochain démarrage." className="sm:col-span-2">
          <Input value={d.image} onChange={(e) => setD({ ...d, image: e.target.value })} className="font-mono" />
        </Field>
      </div>
    </SettingsCard>
  )
}

function Credentials({ resource, db }: { resource: ResourceRef; db: Database }) {
  const kv = KEY_VALUE.has(db.type)
  const mysql = db.type === "mysql" || db.type === "mariadb"
  const [d, setD] = useState({ username: db.username, password: db.password ?? "", root_password: db.root_password ?? "", database_name: db.database_name })
  const save = useResourcePatch(resource, "Identifiants enregistrés")
  const changed = Object.fromEntries(
    Object.entries(d).filter(([k, v]) => v !== ({ username: db.username, password: db.password ?? "", root_password: db.root_password ?? "", database_name: db.database_name } as Record<string, string>)[k]),
  )
  return (
    <SettingsCard
      title="Identifiants"
      description="La plupart des images n'appliquent un nouveau mot de passe qu'à l'initialisation des données ; Redis, KeyDB et Dragonfly le prennent au redémarrage."
      actions={<SaveButton dirty={Object.keys(changed).length > 0} pending={save.isPending} onClick={() => save.mutate(changed)} />}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        {!kv && (
          <Field label="Utilisateur">
            <Input value={d.username} onChange={(e) => setD({ ...d, username: e.target.value })} className="font-mono" />
          </Field>
        )}
        <Field label="Mot de passe" hint={db.password === undefined ? "Masqué : droit « variables » requis pour le lire." : undefined}>
          <Input value={d.password} onChange={(e) => setD({ ...d, password: e.target.value })} type="password" className="font-mono" placeholder={db.password === undefined ? "••••••••" : undefined} />
        </Field>
        {mysql && (
          <Field label="Mot de passe root">
            <Input value={d.root_password} onChange={(e) => setD({ ...d, root_password: e.target.value })} type="password" className="font-mono" />
          </Field>
        )}
        {!kv && (
          <Field label="Base initiale">
            <Input value={d.database_name} onChange={(e) => setD({ ...d, database_name: e.target.value })} className="font-mono" />
          </Field>
        )}
      </div>
    </SettingsCard>
  )
}

function Access({ resource, db }: { resource: ResourceRef; db: Database }) {
  const [pub, setPub] = useState(db.is_public)
  const [port, setPort] = useState(db.public_port ? String(db.public_port) : "")
  const save = useResourcePatch(resource, "Accès enregistré")
  const dirty = pub !== db.is_public || port !== (db.public_port ? String(db.public_port) : "")
  return (
    <SettingsCard
      title="Accès"
      description="L'accès public est appliqué aussitôt si la base tourne."
      actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ is_public: pub, public_port: port ? Number(port) : null })} />}
    >
      <div className="space-y-4">
        <Field label="Adresse interne" hint="Joignable par les ressources du même réseau Docker.">
          <CopyInput value={db.internal_url ?? `${db.internal_host}:${db.internal_port}`} secret={!!db.internal_url} />
        </Field>
        <SwitchRow label="Accessible publiquement" hint="Expose la base sur l'IP du serveur par un proxy dédié." checked={pub} onChange={setPub} />
        {pub && (
          <div className="grid gap-4 sm:grid-cols-[10rem_1fr]">
            <Field label="Port public">
              <Input value={port} onChange={(e) => setPort(e.target.value.replace(/\D/g, ""))} className="font-mono" placeholder="5432" />
            </Field>
            {db.public_url && (
              <Field label="Adresse publique">
                <CopyInput value={db.public_url} secret />
              </Field>
            )}
          </div>
        )}
      </div>
    </SettingsCard>
  )
}

function Tls({ resource, db }: { resource: ResourceRef; db: Database }) {
  const modes = SSL_MODES[db.type]
  const initial = { enable_ssl: Boolean(db.settings.enable_ssl), ssl_mode: String(db.settings.ssl_mode ?? modes?.[0] ?? "") }
  const [d, setD] = useState(initial)
  const save = useResourcePatch(resource, "TLS enregistré (appliqué au prochain démarrage)")
  const dirty = d.enable_ssl !== initial.enable_ssl || d.ssl_mode !== initial.ssl_mode
  return (
    <SettingsCard title="TLS" description="Certificat signé par l'autorité du serveur, régénéré à chaque démarrage." actions={<SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ settings: d })} />}>
      <SwitchRow label="Chiffrer les connexions" checked={d.enable_ssl} onChange={(v) => setD({ ...d, enable_ssl: v })} />
      {d.enable_ssl && modes && (
        <Field label="Mode" className="max-w-xs">
          <Select value={d.ssl_mode} onValueChange={(v) => setD({ ...d, ssl_mode: v })}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {modes.map((m) => (
                <SelectItem key={m} value={m}>
                  {m}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}
      {db.ca_certificate && (
        <Field label="Certificat de l'autorité" className="mt-4">
          <Textarea value={db.ca_certificate} readOnly rows={5} className="font-mono" />
        </Field>
      )}
    </SettingsCard>
  )
}

function CustomConfig({ resource, db }: { resource: ResourceRef; db: Database }) {
  const [config, setConfig] = useState(db.custom_config)
  const save = useResourcePatch(resource, "Configuration enregistrée (appliquée au prochain démarrage)")
  return (
    <SettingsCard
      title="Configuration personnalisée"
      description={`Contenu de ${CONFIG_FILE[db.type]} (64 Ko max.).`}
      actions={<SaveButton dirty={config !== db.custom_config} pending={save.isPending} onClick={() => save.mutate({ custom_config: config })} />}
    >
      <Textarea value={config} onChange={(e) => setConfig(e.target.value)} rows={10} className="font-mono" spellCheck={false} />
    </SettingsCard>
  )
}

function InitScripts({ resource, db }: { resource: ResourceRef; db: Database }) {
  const initial = db.init_scripts ?? []
  const [scripts, setScripts] = useState(initial)
  const save = useResourcePatch(resource, "Scripts enregistrés")
  const dirty = JSON.stringify(scripts) !== JSON.stringify(initial)
  return (
    <SettingsCard
      title="Scripts d'initialisation"
      description="Exécutés par l'image au premier démarrage seulement (1 Mo au total)."
      actions={
        <>
          <Button size="sm" variant="ghost" onClick={() => setScripts([...scripts, { filename: `init-${scripts.length + 1}.sql`, content: "" }])}>
            <PlusIcon /> Ajouter
          </Button>
          <SaveButton dirty={dirty} pending={save.isPending} onClick={() => save.mutate({ init_scripts: scripts })} />
        </>
      }
    >
      <div className="space-y-4">
        {scripts.map((s, i) => (
          <div key={i} className="space-y-2">
            <div className="flex gap-2">
              <Input value={s.filename} onChange={(e) => setScripts(scripts.map((x, j) => (j === i ? { ...x, filename: e.target.value } : x)))} className="font-mono" />
              <Button size="sm" variant="ghost" aria-label="Retirer" onClick={() => setScripts(scripts.filter((_, j) => j !== i))}>
                <Trash2Icon />
              </Button>
            </div>
            <Textarea value={s.content} onChange={(e) => setScripts(scripts.map((x, j) => (j === i ? { ...x, content: e.target.value } : x)))} rows={6} className="font-mono" spellCheck={false} />
          </div>
        ))}
        {scripts.length === 0 && <p className="text-sm text-muted-foreground">Aucun script.</p>}
      </div>
    </SettingsCard>
  )
}

function Container({ resource, db }: { resource: ResourceRef; db: Database }) {
  const initial = {
    ports_mappings: db.ports_mappings,
    custom_docker_run_options: db.custom_docker_run_options,
    health_check_enabled: db.settings.health_check_enabled !== false,
    stop_grace_period: String(db.settings.stop_grace_period ?? 30),
  }
  const [d, setD] = useState(initial)
  const save = useResourcePatch(resource, "Conteneur enregistré (appliqué au prochain démarrage)")
  const dirty = JSON.stringify(d) !== JSON.stringify(initial)
  return (
    <SettingsCard
      title="Conteneur"
      actions={
        <SaveButton
          dirty={dirty}
          pending={save.isPending}
          onClick={() =>
            save.mutate({
              ports_mappings: d.ports_mappings,
              custom_docker_run_options: d.custom_docker_run_options,
              settings: { health_check_enabled: d.health_check_enabled, stop_grace_period: Number(d.stop_grace_period) },
            })
          }
        />
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Ports publiés" hint="Ex. « 5433:5432 », séparés par des virgules.">
          <Input value={d.ports_mappings} onChange={(e) => setD({ ...d, ports_mappings: e.target.value })} className="font-mono" />
        </Field>
        <Field label="Délai d'arrêt (s)">
          <Input value={d.stop_grace_period} onChange={(e) => setD({ ...d, stop_grace_period: e.target.value.replace(/\D/g, "") })} className="font-mono" />
        </Field>
        <Field label="Options docker run" hint="Ex. « --shm-size=256m »." className="sm:col-span-2">
          <Input value={d.custom_docker_run_options} onChange={(e) => setD({ ...d, custom_docker_run_options: e.target.value })} className="font-mono" />
        </Field>
      </div>
      <SwitchRow label="Vérification de santé" hint="Propre au type de base ; le démarrage attend qu'elle réussisse." checked={d.health_check_enabled} onChange={(v) => setD({ ...d, health_check_enabled: v })} />
    </SettingsCard>
  )
}
