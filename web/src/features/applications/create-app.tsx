import { useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { BoxIcon, ContainerIcon, FileCodeIcon, GitBranchIcon, KeyRoundIcon, LayersIcon } from "lucide-react"
import { CodeEditor } from "@/components/code-editor"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { post, teamPath } from "@/lib/api"
import { notifyPending, useApiMutation } from "@/lib/mutations"
import type { Application } from "@/lib/types"
import { cn } from "@/lib/utils"
import { useDestinations } from "@/features/resources/locations"
import { keysQuery } from "@/features/servers/queries"
import { sourcesQuery } from "@/features/sources/queries"
import { RepoPicker } from "@/features/sources/repo-picker"
import { BUILD_PACKS } from "./queries"

type Origin = "public" | "source" | "deploykey" | "image" | "compose" | "dockerfile"

const ORIGINS: { id: Origin; title: string; text: string; icon: typeof BoxIcon }[] = [
  { id: "public", title: "Dépôt public", text: "URL https d'un dépôt Git public.", icon: GitBranchIcon },
  { id: "source", title: "Source Git", text: "Dépôt privé via GitHub App, GitLab ou Gitea.", icon: GitBranchIcon },
  { id: "deploykey", title: "Clé de déploiement", text: "Dépôt privé cloné en SSH avec une clé.", icon: KeyRoundIcon },
  { id: "image", title: "Image Docker", text: "Image d'un registre, sans build.", icon: ContainerIcon },
  { id: "compose", title: "Docker Compose", text: "docker-compose collé ici.", icon: LayersIcon },
  { id: "dockerfile", title: "Dockerfile", text: "Dockerfile écrit ici, sans dépôt.", icon: FileCodeIcon },
]

export function CreateApplication({ team, environment }: { team: string; environment: string }) {
  const navigate = useNavigate()
  const sources = useQuery(sourcesQuery(team))
  const keys = useQuery(keysQuery(team))
  const dests = useDestinations(team)
  const [origin, setOrigin] = useState<Origin>("public")
  const [f, setF] = useState({
    name: "",
    git_repository: "",
    git_branch: "main",
    source_id: "",
    private_key_id: "",
    build_pack: "nixpacks",
    docker_image: "",
    docker_image_tag: "latest",
    docker_compose_raw: "services:\n  web:\n    image: nginx:alpine\n",
    dockerfile: "FROM nginx:alpine\n",
    ports_exposes: "",
    base_directory: "/",
    destination_id: "",
    domains: "",
  })
  const [deployNow, setDeployNow] = useState(true)
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })
  const destination = f.destination_id || (dests.length === 1 ? dests[0].id : "")

  const create = useApiMutation(
    async () => {
      const body: Record<string, unknown> = { environment_id: environment, destination_id: destination, name: f.name.trim() }
      if (origin === "public" || origin === "source" || origin === "deploykey") {
        Object.assign(body, { git_repository: f.git_repository.trim(), git_branch: f.git_branch, build_pack: f.build_pack, base_directory: f.base_directory })
        if (origin === "source") body.source_id = f.source_id
        if (origin === "deploykey") body.private_key_id = f.private_key_id
      }
      if (origin === "image") Object.assign(body, { build_pack: "dockerimage", docker_image: f.docker_image.trim(), docker_image_tag: f.docker_image_tag || "latest" })
      if (origin === "compose") Object.assign(body, { build_pack: "dockercompose", docker_compose_raw: f.docker_compose_raw })
      if (origin === "dockerfile") Object.assign(body, { build_pack: "dockerfile", dockerfile: f.dockerfile })
      if (f.ports_exposes) body.ports_exposes = f.ports_exposes
      const domains = f.domains.split(/[\s,]+/).filter(Boolean)
      if (domains.length) body.domains = domains
      const app = await post<Application>(teamPath(team, "/applications"), body)
      if (deployNow) notifyPending(await post(teamPath(team, `/applications/${app.id}/deploy`)).catch(() => undefined))
      return app
    },
    {
      success: deployNow ? "Application créée, déploiement lancé" : "Application créée",
      invalidate: [["team", team, "resources"]],
      onSuccess: (app) => void navigate({ to: deployNow ? "/t/$team/applications/$app/deployments" : "/t/$team/applications/$app", params: { team, app: app.id } }),
    },
  )

  const gitBased = origin === "public" || origin === "source" || origin === "deploykey"
  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate(undefined)
      }}
    >
      <Card>
        <CardHeader>
          <CardTitle>1. Origine du code</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {ORIGINS.map((o) => {
            const Icon = o.icon
            return (
              <button
                key={o.id}
                type="button"
                onClick={() => setOrigin(o.id)}
                className={cn("flex items-start gap-3 rounded-lg border p-3 text-left transition-colors hover:bg-accent/40", origin === o.id && "border-primary ring-1 ring-primary")}
              >
                <Icon className="mt-0.5 size-4 text-muted-foreground" />
                <span>
                  <span className="block text-sm font-medium">{o.title}</span>
                  <span className="block text-xs text-muted-foreground">{o.text}</span>
                </span>
              </button>
            )
          })}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>2. Configuration</CardTitle>
          <CardDescription>Tout reste modifiable après la création.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <Field label="Nom" className="md:col-span-2">
            <Input required value={f.name} onChange={set("name")} placeholder="ex. site-vitrine" />
          </Field>
          {origin === "public" && (
            <>
              <Field label="URL du dépôt">
                <Input required value={f.git_repository} onChange={set("git_repository")} placeholder="https://github.com/acme/site" />
              </Field>
              <Field label="Branche">
                <Input value={f.git_branch} onChange={set("git_branch")} />
              </Field>
            </>
          )}
          {origin === "source" && (
            <>
              <Field label="Source Git" className="md:col-span-2">
                <Select value={f.source_id} onValueChange={(v) => setF({ ...f, source_id: v, git_repository: "" })}>
                  <SelectTrigger>
                    <SelectValue placeholder={sources.data?.length ? "Choisir une source" : "Aucune source : ajoutez-en une (Sources Git)"} />
                  </SelectTrigger>
                  <SelectContent>
                    {sources.data?.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              {f.source_id && (
                <div className="md:col-span-2">
                  <RepoPicker
                    team={team}
                    source={f.source_id}
                    repo={f.git_repository}
                    branch={f.git_branch}
                    onChange={({ repo, branch }) => setF({ ...f, git_repository: repo, git_branch: branch })}
                  />
                </div>
              )}
            </>
          )}
          {origin === "deploykey" && (
            <>
              <Field label="Dépôt (SSH)" hint="ex. git@github.com:acme/site.git">
                <Input required value={f.git_repository} onChange={set("git_repository")} />
              </Field>
              <Field label="Branche">
                <Input value={f.git_branch} onChange={set("git_branch")} />
              </Field>
              <Field label="Clé de déploiement" className="md:col-span-2" hint="Ajoutez sa clé publique aux deploy keys du dépôt (Serveurs → Clés SSH).">
                <Select value={f.private_key_id} onValueChange={(v) => setF({ ...f, private_key_id: v })}>
                  <SelectTrigger>
                    <SelectValue placeholder="Choisir une clé" />
                  </SelectTrigger>
                  <SelectContent>
                    {keys.data?.map((k) => (
                      <SelectItem key={k.id} value={k.id}>
                        {k.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </>
          )}
          {gitBased && (
            <>
              <Field label="Build pack" className="md:col-span-2">
                <div className="grid gap-2 sm:grid-cols-3">
                  {(["nixpacks", "dockerfile", "dockercompose", "static"] as const).map((bp) => (
                    <button
                      key={bp}
                      type="button"
                      onClick={() => setF({ ...f, build_pack: bp })}
                      className={cn("rounded-lg border p-2.5 text-left transition-colors hover:bg-accent/40", f.build_pack === bp && "border-primary ring-1 ring-primary")}
                    >
                      <span className="block text-sm font-medium">{BUILD_PACKS[bp].label}</span>
                      <span className="block text-xs text-muted-foreground">{BUILD_PACKS[bp].text}</span>
                    </button>
                  ))}
                </div>
              </Field>
              <Field label="Dossier de base" hint="Sous-dossier du dépôt (monorepo).">
                <Input className="font-mono" value={f.base_directory} onChange={set("base_directory")} />
              </Field>
            </>
          )}
          {origin === "image" && (
            <>
              <Field label="Image">
                <Input required className="font-mono" value={f.docker_image} onChange={set("docker_image")} placeholder="ghcr.io/acme/api" />
              </Field>
              <Field label="Tag">
                <Input className="font-mono" value={f.docker_image_tag} onChange={set("docker_image_tag")} />
              </Field>
            </>
          )}
          {origin === "compose" && (
            <Field label="docker-compose.yml" className="md:col-span-2" hint="Les domaines par service se règlent ensuite (onglet Général).">
              <CodeEditor language="yaml" value={f.docker_compose_raw} onChange={(v) => setF({ ...f, docker_compose_raw: v })} minHeight="220px" />
            </Field>
          )}
          {origin === "dockerfile" && (
            <Field label="Dockerfile" className="md:col-span-2">
              <CodeEditor value={f.dockerfile} onChange={(v) => setF({ ...f, dockerfile: v })} minHeight="220px" />
            </Field>
          )}
          {origin !== "compose" && f.build_pack !== "dockercompose" && (
            <>
              <Field label="Port exposé" hint="Port écouté par l'application (3000 pour Nixpacks, 80 sinon).">
                <Input className="font-mono" value={f.ports_exposes} onChange={set("ports_exposes")} placeholder="3000" />
              </Field>
              <Field label="Domaines" hint="Vide : domaine généré (sslip.io ou domaine joker du serveur).">
                <Input value={f.domains} onChange={set("domains")} placeholder="https://app.example.com" />
              </Field>
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>3. Serveur</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field label="Destination (serveur et réseau Docker)">
            <Select value={destination} onValueChange={(v) => setF({ ...f, destination_id: v })}>
              <SelectTrigger>
                <SelectValue placeholder={dests.length ? "Choisir" : "Aucun serveur : ajoutez-en un"} />
              </SelectTrigger>
              <SelectContent>
                {dests.map((d) => (
                  <SelectItem key={d.id} value={d.id} disabled={!d.server.is_usable}>
                    {d.server.name} · {d.network}
                    {!d.server.is_usable && " (non validé)"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Label className="font-normal">
            <Checkbox checked={deployNow} onCheckedChange={(c) => setDeployNow(c === true)} /> Déployer immédiatement
          </Label>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button type="submit" size="lg" loading={create.isPending} disabled={!destination}>
          Créer l'application
        </Button>
      </div>
    </form>
  )
}
