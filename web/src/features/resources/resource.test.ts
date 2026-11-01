import { describe, expect, it } from "vitest"
import { envLabel } from "@/features/services/service-config"
import { resourceApi, resourceMenu, resourcePage } from "./resource"

describe("ressources", () => {
  const ref = { team: "t1", kind: "database" as const, id: "d1" }

  it("construit les chemins de l'API et du front", () => {
    expect(resourceApi(ref, "/envs")).toBe("/teams/t1/databases/d1/envs")
    expect(resourcePage(ref, "/logs")).toBe("/t/t1/databases/d1/logs")
  })

  it("propose les onglets propres à chaque type", () => {
    const labels = (kind: "application" | "database" | "service") => resourceMenu({ ...ref, kind }).groups.flatMap((g) => g.items.map((i) => i.label))
    expect(labels("database")).toContain("Sauvegardes")
    expect(labels("service")).toContain("Domaines")
    expect(labels("application")).toContain("Déploiements")
    expect(labels("service")).not.toContain("Déploiements")
  })

  it("rend lisibles les variables générées, comme Coolify", () => {
    expect(envLabel("SERVICE_PASSWORD_ADMIN")).toBe("Admin · Password")
    expect(envLabel("SERVICE_USER_MARIADB")).toBe("MariaDB · User")
    expect(envLabel("MARIADB_DATABASE")).toBe("MariaDB Database")
  })
})
