import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { DatabaseIcon } from "lucide-react"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import type { Database } from "@/lib/types"
import { cn } from "@/lib/utils"
import { useDestinations } from "@/features/resources/locations"
import { DB_TYPES } from "./queries"

/** Assistant de création d'une base de données (F9-01). */
export function CreateDatabase({ team, environment }: { team: string; environment: string }) {
  const navigate = useNavigate()
  const dests = useDestinations(team)
  const [type, setType] = useState<Database["type"]>("postgresql")
  const [f, setF] = useState({ name: "", image: "", username: "", password: "", root_password: "", database_name: "", destination_id: "" })
  const [startNow, setStartNow] = useState(true)
  const t = DB_TYPES[type]
  const destination = f.destination_id || (dests.length === 1 ? dests[0].id : "")
  const create = useApiMutation(
    () => {
      const body: Record<string, unknown> = { environment_id: environment, destination_id: destination, type, name: f.name.trim(), instant_deploy: startNow }
      for (const k of ["image", "username", "password", "root_password", "database_name"] as const) if (f[k].trim()) body[k] = f[k].trim()
      return post<Database>(teamPath(team, "/databases"), body)
    },
    {
      success: "Base créée",
      invalidate: [["team", team, "resources"]],
      onSuccess: (db) => void navigate({ to: "/t/$team/databases/$db", params: { team, db: db.id } }),
    },
  )
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })

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
          <CardTitle>1. Type</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          {(Object.keys(DB_TYPES) as Database["type"][]).map((k) => (
            <button
              key={k}
              type="button"
              onClick={() => setType(k)}
              className={cn("flex items-center gap-3 rounded-lg border p-3 text-left transition-colors hover:bg-accent/40", type === k && "border-primary ring-1 ring-primary")}
            >
              <DatabaseIcon className="size-4 text-muted-foreground" />
              <span>
                <span className="block text-sm font-medium">{DB_TYPES[k].label}</span>
                <span className="block font-mono text-xs text-muted-foreground">{DB_TYPES[k].image.split("/").pop()}</span>
              </span>
            </button>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>2. Configuration</CardTitle>
          <CardDescription>Champs vides : valeurs par défaut, mots de passe générés (32 caractères).</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <Field label="Nom">
            <Input required value={f.name} onChange={set("name")} placeholder={`ex. ${type}-principale`} />
          </Field>
          <Field label="Image">
            <Input className="font-mono" value={f.image} onChange={set("image")} placeholder={t.image} />
          </Field>
          {t.username && (
            <Field label="Utilisateur">
              <Input className="font-mono" value={f.username} onChange={set("username")} placeholder={t.username} autoComplete="off" />
            </Field>
          )}
          {t.database && (
            <Field label="Base initiale">
              <Input className="font-mono" value={f.database_name} onChange={set("database_name")} placeholder={t.database} />
            </Field>
          )}
          <Field label="Mot de passe" hint="8 à 256 caractères.">
            <Input type="password" value={f.password} onChange={set("password")} placeholder="généré" autoComplete="new-password" />
          </Field>
          {t.rootPassword && (
            <Field label="Mot de passe root">
              <Input type="password" value={f.root_password} onChange={set("root_password")} placeholder="généré" autoComplete="new-password" />
            </Field>
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
                  <SelectItem key={d.id} value={d.id}>
                    {d.server.name} · {d.network}
                    {!d.server.is_usable && " (non validé)"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Label className="font-normal">
            <Checkbox checked={startNow} onCheckedChange={(c) => setStartNow(c === true)} /> Démarrer immédiatement
          </Label>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button type="submit" size="lg" loading={create.isPending} disabled={!destination}>
          Créer la base
        </Button>
      </div>
    </form>
  )
}
