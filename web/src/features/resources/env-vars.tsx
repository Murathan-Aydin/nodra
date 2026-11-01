import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { EyeIcon, EyeOffIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { api, get, post } from "@/lib/api"
import type { EnvVar } from "@/lib/types"
import { resourceApi, resourceKey, type ResourceRef } from "./resource"

type Flags = Pick<EnvVar, "is_secret" | "is_literal" | "is_multiline"> & { is_buildtime?: boolean; is_preview?: boolean }

/** Onglet « Variables d'environnement » (environment-variables du fork) : liste éditable et vue .env. */
export function EnvVarsTab({ resource, buildtime = false }: { resource: ResourceRef; buildtime?: boolean }) {
  const qc = useQueryClient()
  const key = resourceKey(resource, "envs")
  const envs = useQuery({ queryKey: key, queryFn: () => get<EnvVar[]>(resourceApi(resource, "/envs")) })
  const [dev, setDev] = useState(false)
  const refresh = (list: EnvVar[]) => qc.setQueryData(key, list)
  const list = (envs.data ?? []).filter((v) => !(v as EnvVar & { is_preview?: boolean }).is_preview)

  return (
    <div className="space-y-6">
      <SettingsCard
        title="Variables d'environnement"
        description="Valeurs chiffrées. Elles peuvent utiliser les variables partagées : {{team.X}}, {{project.X}}, {{environment.X}}, {{server.X}}."
        actions={
          <Button size="sm" variant="outline" onClick={() => setDev(!dev)}>
            {dev ? "Vue normale" : "Vue développeur"}
          </Button>
        }
      >
        {envs.isPending ? (
          <Skeleton className="h-32" />
        ) : dev ? (
          <BulkEditor resource={resource} vars={list} onSaved={(l) => (refresh(l), setDev(false))} />
        ) : (
          <div className="space-y-2">
            {list.map((v) => (
              <EnvRow key={v.id} resource={resource} env={v} onSaved={refresh} buildtime={buildtime} />
            ))}
            {list.length === 0 && <p className="py-2 text-sm text-muted-foreground">Aucune variable.</p>}
            <NewEnv resource={resource} onSaved={refresh} buildtime={buildtime} />
          </div>
        )}
      </SettingsCard>
    </div>
  )
}

function EnvRow({ resource, env, onSaved, buildtime }: { resource: ResourceRef; env: EnvVar & Flags; onSaved: (l: EnvVar[]) => void; buildtime: boolean }) {
  const [value, setValue] = useState(env.value)
  const [flags, setFlags] = useState<Flags>({ is_secret: env.is_secret, is_literal: env.is_literal, is_multiline: env.is_multiline, is_buildtime: env.is_buildtime })
  const [shown, setShown] = useState(false)
  const dirty = value !== env.value || flags.is_secret !== env.is_secret || flags.is_literal !== env.is_literal || flags.is_multiline !== env.is_multiline || flags.is_buildtime !== env.is_buildtime
  const save = useMutation({
    mutationFn: () => api<EnvVar[]>(resourceApi(resource, `/envs/${env.id}`), { method: "PATCH", json: { ...flags, ...(value !== env.value || !env.is_secret ? { value } : {}) } }),
    onSuccess: (l) => (onSaved(l), toast.success(`${env.key} enregistrée`)),
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api<EnvVar[]>(resourceApi(resource, `/envs/${env.id}`), { method: "DELETE" }),
    onSuccess: onSaved,
    onError: (e) => toast.error(e.message),
  })
  const masked = flags.is_secret && !shown
  return (
    <div className="rounded-xl border border-border p-3">
      <div className="grid gap-2 md:grid-cols-[minmax(10rem,18rem)_1fr_auto]">
        <Input value={env.key} readOnly className="font-mono" />
        <div className="relative">
          {flags.is_multiline && !masked ? (
            <Textarea value={value} onChange={(e) => setValue(e.target.value)} className="font-mono" />
          ) : (
            <Input value={value} onChange={(e) => setValue(e.target.value)} type={masked ? "password" : "text"} placeholder={env.is_secret && !env.value ? "Secret : laisser vide pour le conserver" : undefined} className="pr-9 font-mono" />
          )}
          {flags.is_secret && (
            <button type="button" onClick={() => setShown(!shown)} aria-label={shown ? "Masquer" : "Afficher"} className="absolute top-2 right-2 text-muted-foreground hover:text-foreground">
              {shown ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
            </button>
          )}
        </div>
        <div className="flex items-start gap-1">
          <Button size="sm" variant="outline" disabled={!dirty} loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
          <Button size="sm" variant="ghost" aria-label="Supprimer" loading={remove.isPending} onClick={() => remove.mutate()}>
            <Trash2Icon />
          </Button>
        </div>
      </div>
      <FlagBoxes flags={flags} onChange={setFlags} buildtime={buildtime} />
      {env.comment && <p className="mt-2 text-xs text-muted-foreground">{env.comment}</p>}
    </div>
  )
}

function FlagBoxes({ flags, onChange, buildtime }: { flags: Flags; onChange: (f: Flags) => void; buildtime: boolean }) {
  const boxes: [keyof Flags, string][] = [
    ...(buildtime ? ([["is_buildtime", "Disponible au build"]] as [keyof Flags, string][]) : []),
    ["is_literal", "Littérale (pas d'interpolation)"],
    ["is_multiline", "Multiligne"],
    ["is_secret", "Secret"],
  ]
  return (
    <div className="mt-2 flex flex-wrap gap-x-5 gap-y-1">
      {boxes.map(([k, label]) => (
        <label key={k} className="flex items-center gap-2 text-xs text-muted-foreground">
          <Checkbox checked={!!flags[k]} onCheckedChange={(c) => onChange({ ...flags, [k]: c === true })} />
          {label}
        </label>
      ))}
    </div>
  )
}

function NewEnv({ resource, onSaved, buildtime }: { resource: ResourceRef; onSaved: (l: EnvVar[]) => void; buildtime: boolean }) {
  const [key, setKey] = useState("")
  const [value, setValue] = useState("")
  const [flags, setFlags] = useState<Flags>({ is_secret: false, is_literal: false, is_multiline: false, is_buildtime: buildtime })
  const add = useMutation({
    mutationFn: () => post<EnvVar[]>(resourceApi(resource, "/envs"), { key: key.trim(), value, ...flags }),
    onSuccess: (l) => {
      onSaved(l)
      setKey("")
      setValue("")
      toast.success("Variable ajoutée")
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <form
      className="rounded-xl border border-dashed border-border p-3"
      onSubmit={(e) => {
        e.preventDefault()
        if (key.trim()) add.mutate()
      }}
    >
      <div className="grid gap-2 md:grid-cols-[minmax(10rem,18rem)_1fr_auto]">
        <Input value={key} onChange={(e) => setKey(e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "_"))} placeholder="NOUVELLE_VARIABLE" className="font-mono" />
        <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder="valeur" className="font-mono" />
        <Button size="sm" type="submit" disabled={!key.trim()} loading={add.isPending}>
          <PlusIcon /> Ajouter
        </Button>
      </div>
      <FlagBoxes flags={flags} onChange={setFlags} buildtime={buildtime} />
    </form>
  )
}

/** Vue développeur : toutes les variables au format .env (les secrets ne sont pas renvoyés par l'API). */
function BulkEditor({ resource, vars, onSaved }: { resource: ResourceRef; vars: EnvVar[]; onSaved: (l: EnvVar[]) => void }) {
  const [content, setContent] = useState(() => vars.filter((v) => !v.is_secret).map((v) => `${v.key}=${v.is_multiline ? JSON.stringify(v.value) : v.value}`).join("\n"))
  const save = useMutation({
    mutationFn: () => post<EnvVar[]>(resourceApi(resource, "/envs/bulk"), { content }),
    onSuccess: (l) => (onSaved(l), toast.success("Variables enregistrées")),
    onError: (e) => toast.error(e.message),
  })
  const secrets = vars.filter((v) => v.is_secret).length
  return (
    <div className="space-y-3">
      <Textarea value={content} onChange={(e) => setContent(e.target.value)} rows={14} className="font-mono" spellCheck={false} />
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          Une variable par ligne, <code>CLÉ=valeur</code>. Les variables absentes ne sont pas supprimées.
          {secrets > 0 && ` ${secrets} secret(s) masqué(s) non affiché(s).`}
        </p>
        <Button size="sm" loading={save.isPending} onClick={() => save.mutate()}>
          Enregistrer
        </Button>
      </div>
    </div>
  )
}
