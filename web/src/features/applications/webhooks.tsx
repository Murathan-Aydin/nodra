import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm"
import { CopyInput } from "@/components/copy-input"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { get, post } from "@/lib/api"
import { resourceApi, resourceKey, type ResourceRef } from "@/features/resources/resource"

type Hooks = { secret: string; urls: { github: string; gitlab: string; gitea: string; bitbucket: string; deploy: string } }

/** Onglet « Webhooks » (Webhooks du fork) : URL de déploiement pour la CI et webhooks manuels des forges. */
export function WebhooksTab({ resource }: { resource: ResourceRef }) {
  const qc = useQueryClient()
  const key = resourceKey(resource, "webhooks")
  const hooks = useQuery({ queryKey: key, queryFn: () => get<Hooks>(resourceApi(resource, "/webhooks")) })
  const [rotating, setRotating] = useState(false)
  const rotate = useMutation({
    mutationFn: () => post<Hooks>(resourceApi(resource, "/webhooks/rotate")),
    onSuccess: (h) => {
      qc.setQueryData(key, h)
      setRotating(false)
      toast.success("Nouveau secret généré")
    },
    onError: (e) => toast.error(e.message),
  })
  if (!hooks.data) return <Skeleton className="h-64 rounded-2xl" />
  const h = hooks.data
  return (
    <div className="space-y-6">
      <SettingsCard
        title="Déploiement depuis une CI"
        description="GET ou POST avec le secret en en-tête X-Nodra-Secret (ou ?secret=). ?force=1 reconstruit, ?commit=<sha> déploie ce commit."
        actions={
          <Button size="sm" variant="outline" onClick={() => setRotating(true)}>
            Régénérer le secret
          </Button>
        }
      >
        <div className="space-y-4">
          <Field label="URL">
            <CopyInput value={h.urls.deploy} />
          </Field>
          <Field label="Secret">
            <CopyInput value={h.secret} secret />
          </Field>
          <Field label="Exemple">
            <CopyInput value={`curl -X POST -H "X-Nodra-Secret: ${h.secret}" ${h.urls.deploy}`} secret />
          </Field>
        </div>
      </SettingsCard>
      <SettingsCard title="Webhooks manuels des forges" description="Pour un dépôt sans source Git de l'équipe : ajoutez l'URL dans les webhooks du dépôt (événements push et pull request), avec le secret ci-dessus.">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="GitHub">
            <CopyInput value={h.urls.github} />
          </Field>
          <Field label="GitLab">
            <CopyInput value={h.urls.gitlab} />
          </Field>
          <Field label="Gitea / Forgejo">
            <CopyInput value={h.urls.gitea} />
          </Field>
          <Field label="Bitbucket">
            <CopyInput value={h.urls.bitbucket} />
          </Field>
        </div>
      </SettingsCard>
      <ConfirmDialog open={rotating} onOpenChange={setRotating} title="Régénérer le secret ?" description="Les CI et webhooks qui utilisent l'ancien secret ne fonctionneront plus." actionLabel="Régénérer" loading={rotate.isPending} onConfirm={() => rotate.mutate()} />
    </div>
  )
}
