import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { KeyRoundIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { CopyButton } from "@/components/copy-button"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { del, teamPath } from "@/lib/api"
import { relativeTime } from "@/lib/format"
import { useApiMutation } from "@/lib/mutations"
import { usePermission } from "@/lib/permissions"
import type { PrivateKey } from "@/lib/types"
import { KeyForm } from "./key-form"
import { keysQuery } from "./queries"

export function KeysPanel({ team }: { team: string }) {
  const keys = useQuery(keysQuery(team))
  const canManage = usePermission(team, "key.manage")
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<PrivateKey | null>(null)
  const remove = useApiMutation((k: PrivateKey) => del(teamPath(team, `/private-keys/${k.id}`)), {
    success: "Clé supprimée",
    invalidate: [["team", team, "keys"]],
    onSuccess: () => setDeleting(null),
  })
  return (
    <div className="space-y-4">
      {canManage && (
        <div className="flex justify-end">
          <Button onClick={() => setCreating(true)}>
            <PlusIcon /> Nouvelle clé
          </Button>
        </div>
      )}
      {keys.isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : !keys.data?.length ? (
        <EmptyState icon={KeyRoundIcon} title="Aucune clé SSH" description="Les serveurs sont joints en SSH avec une clé générée ou importée ici." />
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {keys.data.map((k) => (
            <Card key={k.id}>
              <CardContent className="space-y-2 pt-5">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="font-medium">{k.name}</p>
                    <p className="truncate font-mono text-xs text-muted-foreground">{k.fingerprint}</p>
                  </div>
                  <div className="flex">
                    <CopyButton value={k.public_key} label="Copier la clé publique" />
                    {canManage && (
                      <Button variant="ghost" size="icon-sm" onClick={() => setDeleting(k)} aria-label="Supprimer">
                        <Trash2Icon />
                      </Button>
                    )}
                  </div>
                </div>
                <code className="block truncate rounded bg-muted/50 px-2 py-1 font-mono text-xs">{k.public_key}</code>
                <p className="text-xs text-muted-foreground">Ajoutée {relativeTime(k.created_at)}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Nouvelle clé SSH</DialogTitle>
            <DialogDescription>Une paire ed25519 est générée si vous n'importez pas de clé.</DialogDescription>
          </DialogHeader>
          <KeyForm team={team} onCreated={() => setCreating(false)} />
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="Supprimer la clé ?"
        description="Impossible si un serveur l'utilise encore."
        confirmLabel="Supprimer"
        destructive
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </div>
  )
}
