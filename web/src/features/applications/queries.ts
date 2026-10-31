import { queryOptions } from "@tanstack/react-query"
import { get, teamPath } from "@/lib/api"
import type { Application, Deployment, ImageInfo, Preview, Tag } from "@/lib/types"

export const appBase = (team: string, app: string) => teamPath(team, `/applications/${app}`)

export const appQuery = (team: string, app: string) =>
  queryOptions({
    queryKey: ["team", team, "app", app],
    queryFn: () => get<{ application: Application; tags: Tag[]; server_id: string; project_id: string }>(appBase(team, app)),
  })

export const deploymentsQuery = (team: string, app: string) =>
  queryOptions({ queryKey: ["team", team, "app", app, "deployments"], queryFn: () => get<Deployment[]>(`${appBase(team, app)}/deployments`) })

export const imagesQuery = (team: string, app: string) =>
  queryOptions({ queryKey: ["team", team, "app", app, "images"], queryFn: () => get<ImageInfo[]>(`${appBase(team, app)}/images`), retry: false })

export const previewsQuery = (team: string, app: string) =>
  queryOptions({ queryKey: ["team", team, "app", app, "previews"], queryFn: () => get<Preview[]>(`${appBase(team, app)}/previews`) })

export const BUILD_PACKS: Record<string, { label: string; text: string }> = {
  nixpacks: { label: "Nixpacks", text: "Détection automatique du langage (Node, Python, Go, PHP…)." },
  dockerfile: { label: "Dockerfile", text: "Dockerfile du dépôt ou écrit ici." },
  dockercompose: { label: "Docker Compose", text: "Plusieurs services décrits par un docker-compose." },
  dockerimage: { label: "Image Docker", text: "Image existante d'un registre (publique ou privée)." },
  static: { label: "Site statique", text: "Fichiers servis par nginx (build facultatif)." },
}
