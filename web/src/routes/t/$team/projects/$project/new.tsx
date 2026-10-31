import { createFileRoute, redirect } from "@tanstack/react-router"
import { projectQueries } from "@/lib/queries"

// « + » d'un projet : nouvelle ressource dans son premier environnement (production).
export const Route = createFileRoute("/t/$team/projects/$project/new")({
  beforeLoad: async ({ context, params }) => {
    const { environments } = await context.queryClient.ensureQueryData(projectQueries.project(params.team, params.project))
    const env = environments.find((e) => e.name === "production") ?? environments[0]
    throw redirect(
      env
        ? { to: "/t/$team/projects/$project/$env/new", params: { ...params, env: env.id } }
        : { to: "/t/$team/projects/$project", params },
    )
  },
})
