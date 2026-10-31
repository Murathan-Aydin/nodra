import { useRef, useState } from "react"
import { UploadIcon } from "lucide-react"
import { toast } from "sonner"
import { Field } from "@/components/field"
import { TaskDialog } from "@/components/task-output"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { buildURL } from "@/lib/api"
import { notifyPending } from "@/lib/mutations"
import type { ServerTask } from "@/lib/types"
import { dbBase } from "./queries"

/** Import d'un dump envoyé en flux (corps brut), avec progression, puis suivi de la restauration (F9-05). */
export function ImportDump({ team, db, accept }: { team: string; db: string; accept: string }) {
  const input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [database, setDatabase] = useState("")
  const [progress, setProgress] = useState<number | null>(null)
  const [task, setTask] = useState<ServerTask | null>(null)

  const upload = () => {
    if (!file) return
    const xhr = new XMLHttpRequest()
    xhr.open("POST", buildURL(`${dbBase(team, db)}/import`, { filename: file.name, database: database || undefined }))
    xhr.setRequestHeader("X-Requested-With", "nodra")
    xhr.setRequestHeader("Content-Type", "application/octet-stream")
    xhr.upload.onprogress = (e) => e.lengthComputable && setProgress(Math.round((e.loaded / e.total) * 100))
    xhr.onload = () => {
      setProgress(null)
      let body: unknown
      try {
        body = JSON.parse(xhr.responseText)
      } catch {
        body = {}
      }
      if (xhr.status >= 400) {
        toast.error((body as { error?: string }).error ?? "Import impossible")
        return
      }
      setFile(null)
      if (input.current) input.current.value = ""
      if (!notifyPending(body)) setTask(body as ServerTask)
    }
    xhr.onerror = () => {
      setProgress(null)
      toast.error("Envoi interrompu")
    }
    setProgress(0)
    xhr.send(file)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Importer un dump</CardTitle>
        <CardDescription>Fichier envoyé au serveur puis restauré dans la base en cours d'exécution (20 Go max.). Formats : {accept.replaceAll(",", ", ")}.</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 md:grid-cols-[1fr_14rem_auto] md:items-end">
        <Field label="Fichier">
          <Input ref={input} type="file" accept={accept} onChange={(e) => setFile(e.target.files?.[0] ?? null)} disabled={progress !== null} />
        </Field>
        <Field label="Base cible" hint="Vide : base par défaut.">
          <Input className="font-mono" value={database} onChange={(e) => setDatabase(e.target.value)} disabled={progress !== null} />
        </Field>
        <Button onClick={upload} disabled={!file} loading={progress !== null}>
          <UploadIcon /> {progress !== null ? `${progress} %` : "Importer"}
        </Button>
        {progress !== null && (
          <div className="h-1.5 overflow-hidden rounded-full bg-muted md:col-span-3">
            <div className="h-full bg-primary transition-[width]" style={{ width: `${progress}%` }} />
          </div>
        )}
      </CardContent>
      <TaskDialog team={team} task={task} onOpenChange={(o) => !o && setTask(null)} />
    </Card>
  )
}
