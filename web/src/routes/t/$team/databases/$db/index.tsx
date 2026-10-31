import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"
import { DownloadIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { CodeEditor } from "@/components/code-editor"
import { CopyButton } from "@/components/copy-button"
import { Field } from "@/components/field"
import { SecretField } from "@/components/secret-field"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { patch } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Database } from "@/lib/types"
import { DB_TYPES, dbBase, dbQuery } from "@/features/databases/queries"

export const Route = createFileRoute("/t/$team/databases/$db/")({ component: DatabaseGeneral })

function DatabaseGeneral() {
  const { team, db } = Route.useParams()
  const { data } = useQuery(dbQuery(team, db))
  if (!data) return null
  return (
    <div className="space-y-6">
      <Connection d={data.database} />
      <DatabaseForm key={data.database.updated_at} team={team} d={data.database} />
    </div>
  )
}

function Connection({ d }: { d: Database }) {
  const t = DB_TYPES[d.type]
  return (
    <Card>
      <CardHeader>
        <CardTitle>Connexion</CardTitle>
        <CardDescription>L'hôte interne est joignable par les ressources du même réseau Docker.</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 md:grid-cols-2">
        <Field label="Hôte interne">
          <Plain value={`${d.internal_host}:${d.internal_port}`} />
        </Field>
        {t.username && (
          <Field label="Utilisateur">
            <Plain value={d.username} />
          </Field>
        )}
        {d.internal_url === undefined ? (
          <p className="text-sm text-muted-foreground md:col-span-2">Identifiants masqués : la permission « Lire les variables » est nécessaire.</p>
        ) : (
          <>
            <Field label="Mot de passe">
              <SecretField value={d.password ?? ""} />
            </Field>
            {d.root_password && (
              <Field label="Mot de passe root">
                <SecretField value={d.root_password} />
              </Field>
            )}
            <Field label="URL interne" className="md:col-span-2">
              <SecretField value={d.internal_url} />
            </Field>
            {d.public_url && (
              <Field label="URL publique" className="md:col-span-2">
                <SecretField value={d.public_url} />
              </Field>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}

function Plain({ value }: { value: string }) {
  return (
    <div className="flex h-9 items-center gap-2 rounded-md border px-3">
      <code className="flex-1 truncate font-mono text-sm">{value}</code>
      <CopyButton value={value} />
    </div>
  )
}

function DatabaseForm({ team, d }: { team: string; d: Database }) {
  const scope = { type: "resource", id: d.id }
  const canUpdate = usePermission(team, "resource.update", scope)
  const canWriteSecrets = usePermission(team, "resource.env.write", scope)
  const t = DB_TYPES[d.type]
  const [f, setF] = useState({
    name: d.name,
    description: d.description,
    image: d.image,
    username: d.username,
    database_name: d.database_name,
    custom_config: d.custom_config,
    init_scripts: d.init_scripts ?? [],
    is_public: d.is_public,
    public_port: d.public_port ? String(d.public_port) : "",
    limits: d.limits,
    settings: d.settings,
  })
  const [pw, setPw] = useState({ password: "", root_password: "" })
  const save = useApiMutation(
    () => {
      const body: Record<string, unknown> = { ...f, public_port: f.public_port ? Number(f.public_port) : null }
      if (!t.username) delete body.username
      if (pw.password) body.password = pw.password
      if (pw.root_password) body.root_password = pw.root_password
      return patch<Database>(dbBase(team, d.id), body)
    },
    { success: "Configuration enregistrée (appliquée au prochain démarrage)", invalidate: [["team", team, "db", d.id]] },
  )
  const set = (k: "name" | "description" | "image" | "username" | "database_name") => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })
  const st = <K extends keyof Database["settings"]>(k: K, v: Database["settings"][K]) => setF({ ...f, settings: { ...f.settings, [k]: v } })
  const disabled = !canUpdate

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(undefined)
      }}
    >
      <fieldset disabled={disabled} className="space-y-6">
        <Section title="Configuration" description="Les identifiants ne sont appliqués par la plupart des images qu'à l'initialisation des données.">
          <Field label="Nom">
            <Input value={f.name} onChange={set("name")} />
          </Field>
          <Field label="Description">
            <Input value={f.description} onChange={set("description")} />
          </Field>
          <Field label="Image">
            <Input className="font-mono" value={f.image} onChange={set("image")} />
          </Field>
          {t.username && (
            <Field label="Utilisateur">
              <Input className="font-mono" value={f.username} onChange={set("username")} autoComplete="off" />
            </Field>
          )}
          {t.database && (
            <Field label="Base initiale">
              <Input className="font-mono" value={f.database_name} onChange={set("database_name")} />
            </Field>
          )}
          {canWriteSecrets && (
            <>
              <Field label="Nouveau mot de passe" hint="Laisser vide pour le conserver.">
                <Input type="password" value={pw.password} onChange={(e) => setPw({ ...pw, password: e.target.value })} autoComplete="new-password" />
              </Field>
              {t.rootPassword && (
                <Field label="Nouveau mot de passe root">
                  <Input type="password" value={pw.root_password} onChange={(e) => setPw({ ...pw, root_password: e.target.value })} autoComplete="new-password" />
                </Field>
              )}
            </>
          )}
        </Section>

        <Section title="Accès public" description="Publie la base sur un port du serveur via un proxy TCP (appliqué aussitôt si elle tourne).">
          <Label className="flex items-center justify-between font-normal md:col-span-2">
            Rendre la base publique
            <Switch checked={f.is_public} onCheckedChange={(v) => setF({ ...f, is_public: v })} disabled={disabled} />
          </Label>
          {f.is_public && (
            <Field label="Port public" hint="Hors 80 et 443.">
              <Input type="number" min={1} max={65535} value={f.public_port} onChange={(e) => setF({ ...f, public_port: e.target.value })} />
            </Field>
          )}
        </Section>

        {t.sslModes && (
          <Section title="TLS" description="Certificat signé par l'autorité du serveur, régénéré à chaque démarrage.">
            <Label className="flex items-center justify-between font-normal md:col-span-2">
              Chiffrer les connexions
              <Switch checked={f.settings.enable_ssl} onCheckedChange={(v) => st("enable_ssl", v)} disabled={disabled} />
            </Label>
            {f.settings.enable_ssl && t.sslModes.length > 0 && (
              <Field label="Mode">
                <Select value={f.settings.ssl_mode || t.sslModes[0]} onValueChange={(v) => st("ssl_mode", v)} disabled={disabled}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {t.sslModes.map((m) => (
                      <SelectItem key={m} value={m}>
                        {m}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            )}
            {d.ca_certificate && (
              <div className="flex items-end">
                <Button type="button" variant="outline" onClick={() => download(`${d.name}-ca.crt`, d.ca_certificate ?? "")}>
                  <DownloadIcon /> Certificat de l'autorité
                </Button>
              </div>
            )}
          </Section>
        )}

        {t.config && (
          <Section title={`Configuration personnalisée (${t.config})`} description="64 Ko maximum ; vide = configuration de l'image.">
            <div className="md:col-span-2">
              <CodeEditor value={f.custom_config} onChange={(v) => setF({ ...f, custom_config: v })} readOnly={disabled} minHeight="160px" />
            </div>
          </Section>
        )}

        {t.initScripts && (
          <Section title="Scripts d'initialisation" description="Exécutés par l'image au premier démarrage uniquement (1 Mo au total).">
            <div className="space-y-4 md:col-span-2">
              {f.init_scripts.map((s, i) => (
                <div key={i} className="space-y-2 rounded-lg border p-3">
                  <div className="flex gap-2">
                    <Input
                      className="font-mono"
                      value={s.filename}
                      placeholder="01-schema.sql"
                      onChange={(e) => setF({ ...f, init_scripts: f.init_scripts.map((x, j) => (j === i ? { ...x, filename: e.target.value } : x)) })}
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label="Retirer"
                      onClick={() => setF({ ...f, init_scripts: f.init_scripts.filter((_, j) => j !== i) })}
                    >
                      <Trash2Icon />
                    </Button>
                  </div>
                  <CodeEditor
                    value={s.content}
                    onChange={(v) => setF({ ...f, init_scripts: f.init_scripts.map((x, j) => (j === i ? { ...x, content: v } : x)) })}
                    readOnly={disabled}
                    minHeight="120px"
                  />
                </div>
              ))}
              {canUpdate && (
                <Button type="button" variant="outline" size="sm" onClick={() => setF({ ...f, init_scripts: [...f.init_scripts, { filename: "", content: "" }] })}>
                  <PlusIcon /> Ajouter un script
                </Button>
              )}
            </div>
          </Section>
        )}

        <Section title="Santé et ressources" description="Limites : 0 ou vide = illimité.">
          <Label className="flex items-center justify-between font-normal md:col-span-2">
            Vérification de santé
            <Switch checked={f.settings.health_check_enabled} onCheckedChange={(v) => st("health_check_enabled", v)} disabled={disabled} />
          </Label>
          <Field label="Mémoire" hint="ex. 512m">
            <Input className="font-mono" value={f.limits.memory} onChange={(e) => setF({ ...f, limits: { ...f.limits, memory: e.target.value } })} />
          </Field>
          <Field label="CPU" hint="ex. 1.5">
            <Input className="font-mono" value={f.limits.cpus} onChange={(e) => setF({ ...f, limits: { ...f.limits, cpus: e.target.value } })} />
          </Field>
          <Field label="Arrêt gracieux (s)">
            <Input type="number" min={0} value={f.settings.stop_grace_period} onChange={(e) => st("stop_grace_period", Number(e.target.value))} />
          </Field>
          {d.type === "postgresql" && (
            <>
              <Field label="POSTGRES_INITDB_ARGS">
                <Input className="font-mono" value={f.settings.postgres_initdb_args} onChange={(e) => st("postgres_initdb_args", e.target.value)} />
              </Field>
              <Field label="POSTGRES_HOST_AUTH_METHOD">
                <Input className="font-mono" value={f.settings.postgres_host_auth_method} onChange={(e) => st("postgres_host_auth_method", e.target.value)} placeholder="scram-sha-256" />
              </Field>
            </>
          )}
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
  )
}

function download(filename: string, content: string) {
  const url = URL.createObjectURL(new Blob([content], { type: "application/x-pem-file" }))
  const a = document.createElement("a")
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
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
