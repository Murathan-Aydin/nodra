import { useState } from "react"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import type { PrivateKey } from "@/lib/types"

/** Génère une paire ed25519 ou importe une clé privée existante. */
export function KeyForm({ team, onCreated }: { team: string; onCreated: (k: PrivateKey) => void }) {
  const [name, setName] = useState("")
  const [mode, setMode] = useState<"generate" | "import">("generate")
  const [privateKey, setPrivateKey] = useState("")
  const create = useApiMutation(
    (v: { name: string; private_key?: string }) => post<PrivateKey>(teamPath(team, "/private-keys"), v),
    { success: "Clé enregistrée", invalidate: [["team", team, "keys"]], onSuccess: onCreated },
  )
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate({ name: name.trim(), private_key: mode === "import" ? privateKey : undefined })
      }}
    >
      <Field label="Nom de la clé" htmlFor="key-name">
        <Input id="key-name" required value={name} onChange={(e) => setName(e.target.value)} placeholder="ex. production" />
      </Field>
      <div className="flex gap-2">
        <Button type="button" size="sm" variant={mode === "generate" ? "default" : "outline"} onClick={() => setMode("generate")}>
          Générer une clé
        </Button>
        <Button type="button" size="sm" variant={mode === "import" ? "default" : "outline"} onClick={() => setMode("import")}>
          Importer une clé
        </Button>
      </div>
      {mode === "import" && (
        <Field label="Clé privée" htmlFor="key-private" hint="Format PEM ou OpenSSH, sans phrase de passe. Elle est chiffrée et jamais réaffichée.">
          <Textarea
            id="key-private"
            required
            rows={6}
            className="font-mono text-xs"
            placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
            value={privateKey}
            onChange={(e) => setPrivateKey(e.target.value)}
          />
        </Field>
      )}
      <Button type="submit" loading={create.isPending}>
        {mode === "generate" ? "Générer" : "Importer"}
      </Button>
    </form>
  )
}
