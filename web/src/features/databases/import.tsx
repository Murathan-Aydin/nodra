import { useRef, useState } from "react"
import { UploadIcon } from "lucide-react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { SettingsCard } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { API_BASE } from "@/lib/api"
import { bytes } from "@/lib/format"
import { resourceApi, type ResourceRef } from "@/features/resources/resource"

/** Onglet « Importer un dump » (Import Backup du fork) : envoi en flux avec progression, puis restauration. */
export function ImportTab({ resource }: { resource: ResourceRef }) {
  const input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [database, setDatabase] = useState("")
  const [progress, setProgress] = useState<number | null>(null)
  const send = () => {
    if (!file) return
    const q = new URLSearchParams({ filename: file.name })
    if (database) q.set("database", database)
    const xhr = new XMLHttpRequest()
    xhr.open("POST", `${API_BASE}${resourceApi(resource, `/import?${q}`)}`)
    xhr.withCredentials = true
    xhr.setRequestHeader("X-Requested-With", "nodra")
    xhr.upload.onprogress = (e) => e.lengthComputable && setProgress(Math.round((e.loaded / e.total) * 100))
    xhr.onload = () => {
      setProgress(null)
      if (xhr.status >= 200 && xhr.status < 300) {
        const approval = xhr.responseText.includes("approval_request")
        toast.success(approval ? "Import en attente d'approbation" : "Import lancé", { description: "Suivi dans l'historique de la base." })
        setFile(null)
      } else {
        let msg = `Erreur HTTP ${xhr.status}`
        try {
          msg = (JSON.parse(xhr.responseText) as { error?: string }).error ?? msg
        } catch {
          // réponse non JSON
        }
        toast.error(msg)
      }
    }
    xhr.onerror = () => (setProgress(null), toast.error("Envoi interrompu"))
    setProgress(0)
    xhr.send(file)
  }
  return (
    <SettingsCard title="Importer un dump" description="Formats : .dump, .sql, .sql.gz, .archive, .archive.gz (20 Go max.). Les données existantes peuvent être écrasées.">
      <div className="space-y-4">
        <button
          type="button"
          onClick={() => input.current?.click()}
          className="flex w-full flex-col items-center gap-2 rounded-xl border border-dashed border-border px-6 py-10 text-sm text-muted-foreground hover:bg-foreground/[0.03]"
        >
          <UploadIcon className="size-6" />
          {file ? (
            <span className="text-foreground">
              {file.name} · {bytes(file.size)}
            </span>
          ) : (
            "Choisir un fichier"
          )}
        </button>
        <input ref={input} type="file" accept=".dump,.sql,.gz,.archive" hidden onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        <Field label="Base cible" hint="Facultatif : la base initiale sinon." className="max-w-sm">
          <Input value={database} onChange={(e) => setDatabase(e.target.value)} className="font-mono" />
        </Field>
        {progress !== null && (
          <div className="h-2 overflow-hidden rounded-full bg-muted">
            <div className="h-full bg-primary transition-[width]" style={{ width: `${progress}%` }} />
          </div>
        )}
        <Button disabled={!file || progress !== null} onClick={send}>
          {progress !== null ? `Envoi… ${progress} %` : "Importer"}
        </Button>
      </div>
    </SettingsCard>
  )
}
