import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { post, teamPath } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import type { GitSource } from "@/lib/types"
import { cn } from "@/lib/utils"
import { SOURCE_LABEL, SourceIcon } from "./source-icon"

type Kind = "github-auto" | "github" | "gitlab" | "gitea"

const KINDS: { kind: Kind; title: string; text: string; type: string }[] = [
  { kind: "github-auto", title: "GitHub App (automatique)", text: "Nodra crée l'App sur GitHub puis vous l'installez.", type: "github" },
  { kind: "github", title: "GitHub App (existante)", text: "Identifiants d'une App déjà créée.", type: "github" },
  { kind: "gitlab", title: "GitLab", text: "gitlab.com ou auto-hébergé, jeton read_api + read_repository.", type: "gitlab" },
  { kind: "gitea", title: "Gitea / Forgejo", text: "Jeton d'accès de l'instance.", type: "gitea" },
]

/** Soumet le manifeste à GitHub (navigation POST) : GitHub crée l'App puis revient sur Nodra. */
function postManifest(action: string, manifest: string) {
  const form = Object.assign(document.createElement("form"), { method: "POST", action })
  form.appendChild(Object.assign(document.createElement("input"), { type: "hidden", name: "manifest", value: manifest }))
  document.body.appendChild(form)
  form.submit()
}

export function CreateSourceDialog({ team, open, onOpenChange }: { team: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const navigate = useNavigate()
  const [kind, setKind] = useState<Kind>("github-auto")
  const [f, setF] = useState<Record<string, string>>({})
  const [isPublic, setIsPublic] = useState(false)
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setF({ ...f, [k]: e.target.value })
  const create = useApiMutation(
    async () => {
      if (kind === "github-auto") {
        const res = await post<{ source: GitSource; form_action: string; manifest: string }>(teamPath(team, "/sources/github/manifest"), {
          name: f.name,
          organization: f.organization || undefined,
          html_url: f.html_url || undefined,
          is_public: isPublic,
        })
        postManifest(res.form_action, res.manifest)
        return res.source
      }
      const body: Record<string, unknown> = { type: kind, name: f.name, html_url: f.html_url || undefined, api_url: f.api_url || undefined }
      if (kind === "github") {
        Object.assign(body, {
          app_id: Number(f.app_id),
          private_key: f.private_key,
          webhook_secret: f.webhook_secret,
          installation_id: f.installation_id ? Number(f.installation_id) : undefined,
          client_id: f.client_id || undefined,
          client_secret: f.client_secret || undefined,
          organization: f.organization || undefined,
        })
      } else {
        body.access_token = f.access_token
      }
      return post<GitSource>(teamPath(team, "/sources"), body)
    },
    {
      success: kind === "github-auto" ? "Redirection vers GitHub…" : "Source ajoutée",
      invalidate: [["team", team, "sources"]],
      onSuccess: (src) => {
        if (kind === "github-auto") return
        onOpenChange(false)
        void navigate({ to: "/t/$team/sources/$source", params: { team, source: src.id } })
      },
    },
  )
  const text = (k: string, label: string, opts: { placeholder?: string; hint?: string; type?: string; required?: boolean } = {}) => (
    <Field label={label} hint={opts.hint}>
      <Input type={opts.type} required={opts.required} placeholder={opts.placeholder} value={f[k] ?? ""} onChange={set(k)} />
    </Field>
  )
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Nouvelle source Git</DialogTitle>
          <DialogDescription>Les dépôts publics et les clés de déploiement n'ont pas besoin de source.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-2 sm:grid-cols-2">
          {KINDS.map((k) => (
            <button
              type="button"
              key={k.kind}
              onClick={() => setKind(k.kind)}
              className={cn("flex items-start gap-3 rounded-lg border p-3 text-left transition-colors hover:bg-accent/40", kind === k.kind && "border-primary ring-1 ring-primary")}
            >
              <SourceIcon type={k.type} className="size-8" />
              <span>
                <span className="block text-sm font-medium">{k.title}</span>
                <span className="block text-xs text-muted-foreground">{k.text}</span>
              </span>
            </button>
          ))}
        </div>
        <form
          id="source-form"
          className="grid gap-4 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate(undefined)
          }}
        >
          {text("name", "Nom", { required: true, placeholder: kind === "github-auto" ? "ex. acme" : SOURCE_LABEL[kind === "github" ? "github" : kind] })}
          {(kind === "github-auto" || kind === "github") && text("organization", "Organisation GitHub", { hint: "Vide : compte personnel." })}
          {text("html_url", "URL de la forge", {
            placeholder: kind === "gitlab" ? "https://gitlab.com" : kind === "gitea" ? "https://gitea.example.com" : "https://github.com",
            hint: kind === "gitea" ? "Obligatoire pour Gitea." : "Vide : service public.",
          })}
          {kind === "github-auto" && (
            <Label className="font-normal sm:col-span-2">
              <Checkbox checked={isPublic} onCheckedChange={(c) => setIsPublic(c === true)} /> App publique (installable par d'autres comptes)
            </Label>
          )}
          {kind === "github" && (
            <>
              {text("app_id", "App ID", { required: true })}
              {text("installation_id", "Installation ID", { hint: "Après l'installation de l'App." })}
              {text("client_id", "Client ID")}
              {text("client_secret", "Client secret", { type: "password" })}
              {text("webhook_secret", "Secret du webhook", { type: "password", required: true })}
              <Field label="Clé privée de l'App (PEM)" className="sm:col-span-2">
                <Textarea required rows={5} className="font-mono text-xs" value={f.private_key ?? ""} onChange={set("private_key")} />
              </Field>
            </>
          )}
          {(kind === "gitlab" || kind === "gitea") && (
            <>
              {text("access_token", "Jeton d'accès", { type: "password", required: true })}
              {text("api_url", "URL de l'API", { hint: "Vide : déduite de l'URL de la forge." })}
            </>
          )}
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Annuler
          </Button>
          <Button type="submit" form="source-form" loading={create.isPending}>
            {kind === "github-auto" ? "Créer l'App sur GitHub" : "Ajouter"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
