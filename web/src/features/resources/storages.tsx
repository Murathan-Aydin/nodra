import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { FileIcon, FolderIcon, HardDriveIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { api, get, post } from "@/lib/api"
import { cn } from "@/lib/utils"
import { resourceApi, resourceKey, type ResourceRef } from "./resource"

export interface Storage {
  id: string
  kind: "volume" | "bind" | "file"
  name: string
  host_path: string
  mount_path: string
  content: string
}

const KINDS = [
  { kind: "volume", label: "Volume", icon: HardDriveIcon, hint: "Volume Docker nommé, géré par Docker." },
  { kind: "bind", label: "Dossier de l'hôte", icon: FolderIcon, hint: "Monte un dossier du serveur dans le conteneur." },
  { kind: "file", label: "Fichier", icon: FileIcon, hint: "Fichier créé par Nodra (1 Mo max.)." },
] as const

/** Onglet « Stockage persistant » (Persistent Storage du fork) : volumes, dossiers montés et fichiers. */
export function StoragesTab({ resource }: { resource: ResourceRef }) {
  const key = resourceKey(resource, "storages")
  const storages = useQuery({ queryKey: key, queryFn: () => get<Storage[]>(resourceApi(resource, "/storages")) })
  const [adding, setAdding] = useState(false)
  return (
    <SettingsCard
      title="Stockage persistant"
      description="Appliqué au prochain déploiement ou redémarrage."
      actions={
        <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
          <PlusIcon /> Ajouter
        </Button>
      }
      bodyClassName="p-0"
    >
      {adding && (
        <div className="border-b border-border p-5">
          <NewStorage resource={resource} queryKey={key} onDone={() => setAdding(false)} />
        </div>
      )}
      {storages.isPending ? (
        <Skeleton className="m-4 h-20" />
      ) : storages.data?.length ? (
        storages.data.map((s) => <StorageRow key={s.id} resource={resource} storage={s} queryKey={key} />)
      ) : (
        !adding && <p className="px-5 py-8 text-center text-sm text-muted-foreground">Aucun stockage.</p>
      )}
    </SettingsCard>
  )
}

function NewStorage({ resource, queryKey, onDone }: { resource: ResourceRef; queryKey: unknown[]; onDone: () => void }) {
  const qc = useQueryClient()
  const [kind, setKind] = useState<Storage["kind"]>("volume")
  const [d, setD] = useState({ name: "", host_path: "", mount_path: "", content: "" })
  const add = useMutation({
    mutationFn: () => post(resourceApi(resource, "/storages"), { kind, mount_path: d.mount_path, ...(kind === "volume" ? { name: d.name || undefined } : kind === "bind" ? { host_path: d.host_path } : { content: d.content }) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey })
      toast.success("Stockage ajouté")
      onDone()
    },
    onError: (e) => toast.error(e.message),
  })
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        add.mutate()
      }}
    >
      <div className="grid gap-2 sm:grid-cols-3">
        {KINDS.map((k) => (
          <button
            key={k.kind}
            type="button"
            onClick={() => setKind(k.kind)}
            className={cn("rounded-xl border p-3 text-left transition-colors", kind === k.kind ? "border-primary/60 bg-primary/10" : "border-border hover:bg-foreground/[0.03]")}
          >
            <k.icon className="mb-2 size-4 text-muted-foreground" />
            <p className="text-sm font-medium">{k.label}</p>
            <p className="text-xs text-muted-foreground">{k.hint}</p>
          </button>
        ))}
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        {kind === "volume" && (
          <Field label="Nom du volume" hint="Facultatif : généré sinon.">
            <Input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} className="font-mono" />
          </Field>
        )}
        {kind === "bind" && (
          <Field label="Chemin sur le serveur">
            <Input value={d.host_path} onChange={(e) => setD({ ...d, host_path: e.target.value })} placeholder="/data/mon-dossier" className="font-mono" required />
          </Field>
        )}
        <Field label="Chemin dans le conteneur">
          <Input value={d.mount_path} onChange={(e) => setD({ ...d, mount_path: e.target.value })} placeholder="/app/data" className="font-mono" required />
        </Field>
        {kind === "file" && (
          <Field label="Contenu" className="sm:col-span-2">
            <Textarea value={d.content} onChange={(e) => setD({ ...d, content: e.target.value })} rows={8} className="font-mono" />
          </Field>
        )}
      </div>
      <div className="flex gap-2">
        <Button type="submit" size="sm" loading={add.isPending}>
          Ajouter
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDone}>
          Annuler
        </Button>
      </div>
    </form>
  )
}

function StorageRow({ resource, storage, queryKey }: { resource: ResourceRef; storage: Storage; queryKey: unknown[] }) {
  const qc = useQueryClient()
  const [content, setContent] = useState(storage.content)
  const [deleting, setDeleting] = useState(false)
  const kind = KINDS.find((k) => k.kind === storage.kind) ?? KINDS[0]
  const save = useMutation({
    mutationFn: () => api(resourceApi(resource, `/storages/${storage.id}`), { method: "PATCH", json: { content } }),
    onSuccess: () => (void qc.invalidateQueries({ queryKey }), toast.success("Fichier enregistré")),
    onError: (e) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api(resourceApi(resource, `/storages/${storage.id}`), { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey }),
    onError: (e) => toast.error(e.message),
  })
  return (
    <div className="border-b border-border px-4 py-3 last:border-b-0">
      <div className="flex items-center gap-3">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-muted-foreground">
          <kind.icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate font-mono text-sm">{storage.mount_path}</p>
          <p className="truncate text-xs text-muted-foreground">
            {kind.label}
            {storage.kind === "volume" && storage.name && ` · ${storage.name}`}
            {storage.kind === "bind" && ` · ${storage.host_path}`}
          </p>
        </div>
        <Button size="sm" variant="ghost" aria-label="Supprimer" onClick={() => setDeleting(true)}>
          <Trash2Icon />
        </Button>
      </div>
      {storage.kind === "file" && (
        <div className="mt-3 space-y-2">
          <Textarea value={content} onChange={(e) => setContent(e.target.value)} rows={6} className="font-mono" />
          <Button size="sm" variant="outline" disabled={content === storage.content} loading={save.isPending} onClick={() => save.mutate()}>
            Enregistrer
          </Button>
        </div>
      )}
      <ConfirmDialog
        open={deleting}
        onOpenChange={setDeleting}
        title={`Retirer ${storage.mount_path} ?`}
        description="Le montage est retiré au prochain déploiement. Les données d'un volume ne sont pas effacées."
        actionLabel="Retirer"
        loading={remove.isPending}
        onConfirm={() => remove.mutate()}
      />
    </div>
  )
}
