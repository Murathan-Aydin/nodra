import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { FileIcon, FolderIcon, HardDriveIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { CodeEditor } from "@/components/code-editor"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { EmptyState } from "@/components/empty-state"
import { Field } from "@/components/field"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { del, get, patch, post } from "@/lib/api"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { Storage } from "@/lib/types"

const ICON = { volume: HardDriveIcon, bind: FolderIcon, file: FileIcon }
const KIND = { volume: "Volume Docker", bind: "Dossier de l'hôte", file: "Fichier" }

/** Stockages persistants d'une ressource : volumes, dossiers de l'hôte, fichiers montés. */
export function StoragesPanel({ team, base, resource }: { team: string; base: string; resource: string }) {
  const key = ["team", team, "storages", resource]
  const storages = useQuery({ queryKey: key, queryFn: () => get<Storage[]>(`${base}/storages`) })
  const canUpdate = usePermission(team, "resource.update", { type: "resource", id: resource })
  const [draft, setDraft] = useState<{ kind: Storage["kind"]; name: string; host_path: string; mount_path: string; content: string } | null>(null)
  const [editing, setEditing] = useState<Storage | null>(null)
  const [deleting, setDeleting] = useState<Storage | null>(null)
  const create = useApiMutation((d: NonNullable<typeof draft>) => post(`${base}/storages`, d), {
    success: "Stockage ajouté (appliqué au prochain déploiement)",
    invalidate: [key],
    onSuccess: () => setDraft(null),
  })
  const update = useApiMutation((s: Storage) => patch(`${base}/storages/${s.id}`, { content: s.content }), {
    success: "Fichier enregistré",
    invalidate: [key],
    onSuccess: () => setEditing(null),
  })
  const remove = useApiMutation((s: Storage) => del(`${base}/storages/${s.id}`), {
    success: "Stockage retiré",
    invalidate: [key],
    onSuccess: () => setDeleting(null),
  })
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">Données conservées entre les déploiements. Les changements s'appliquent au prochain déploiement.</p>
        {canUpdate && (
          <Button size="sm" onClick={() => setDraft({ kind: "volume", name: "", host_path: "", mount_path: "", content: "" })}>
            <PlusIcon /> Ajouter
          </Button>
        )}
      </div>
      {!storages.data?.length ? (
        <EmptyState icon={HardDriveIcon} title="Aucun stockage persistant" />
      ) : (
        <Card>
          <ul className="divide-y">
            {storages.data.map((s) => {
              const Icon = ICON[s.kind]
              return (
                <li key={s.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
                  <Icon className="size-4 text-muted-foreground" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-mono">{s.mount_path}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {KIND[s.kind]}
                      {s.kind === "volume" && ` · ${s.name}`}
                      {s.kind === "bind" && ` · ${s.host_path}`}
                    </span>
                  </span>
                  {canUpdate && s.kind === "file" && (
                    <Button variant="ghost" size="icon-sm" aria-label="Modifier" onClick={() => setEditing(s)}>
                      <PencilIcon />
                    </Button>
                  )}
                  {canUpdate && (
                    <Button variant="ghost" size="icon-sm" aria-label="Supprimer" onClick={() => setDeleting(s)}>
                      <Trash2Icon />
                    </Button>
                  )}
                </li>
              )
            })}
          </ul>
        </Card>
      )}
      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>Nouveau stockage</DialogTitle>
          </DialogHeader>
          {draft && (
            <div className="space-y-4">
              <Field label="Type">
                <Select value={draft.kind} onValueChange={(v) => setDraft({ ...draft, kind: v as Storage["kind"] })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="volume">Volume Docker</SelectItem>
                    <SelectItem value="bind">Dossier de l'hôte</SelectItem>
                    <SelectItem value="file">Fichier</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Point de montage dans le conteneur">
                <Input className="font-mono" placeholder="/data" value={draft.mount_path} onChange={(e) => setDraft({ ...draft, mount_path: e.target.value })} />
              </Field>
              {draft.kind === "volume" && (
                <Field label="Nom du volume" hint="Vide : nom généré.">
                  <Input className="font-mono" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
                </Field>
              )}
              {draft.kind === "bind" && (
                <Field label="Chemin sur le serveur">
                  <Input className="font-mono" placeholder="/srv/uploads" value={draft.host_path} onChange={(e) => setDraft({ ...draft, host_path: e.target.value })} />
                </Field>
              )}
              {draft.kind === "file" && (
                <Field label="Contenu (1 Mo max.)">
                  <CodeEditor value={draft.content} onChange={(content) => setDraft({ ...draft, content })} minHeight="180px" />
                </Field>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              Annuler
            </Button>
            <Button loading={create.isPending} onClick={() => draft && create.mutate(draft)}>
              Ajouter
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={!!editing} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="font-mono">{editing?.mount_path}</DialogTitle>
            <DialogDescription>Fichier monté dans le conteneur.</DialogDescription>
          </DialogHeader>
          {editing && <CodeEditor value={editing.content} onChange={(content) => setEditing({ ...editing, content })} minHeight="260px" />}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)}>
              Annuler
            </Button>
            <Button loading={update.isPending} onClick={() => editing && update.mutate(editing)}>
              Enregistrer
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Retirer ${deleting?.mount_path} ?`}
        description="Le montage disparaît au prochain déploiement ; un volume Docker n'est pas supprimé du serveur."
        confirmLabel="Retirer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </div>
  )
}
