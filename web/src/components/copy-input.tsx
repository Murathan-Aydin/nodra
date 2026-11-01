import { useState } from "react"
import { CheckIcon, CopyIcon, EyeIcon, EyeOffIcon } from "lucide-react"
import { Input } from "@/components/ui/input"
import { copyText } from "@/lib/format"

/** Champ en lecture seule avec bouton copier (et œil pour une valeur secrète). */
export function CopyInput({ value, secret, className }: { value: string; secret?: boolean; className?: string }) {
  const [shown, setShown] = useState(false)
  const [copied, setCopied] = useState(false)
  return (
    <div className="relative">
      <Input value={value} readOnly type={secret && !shown ? "password" : "text"} className={`pr-18 font-mono ${className ?? ""}`} />
      <div className="absolute inset-y-0 right-1.5 flex items-center gap-0.5">
        <button
          type="button"
          title="Copier"
          onClick={() =>
            void copyText(value).then((ok) => {
              if (!ok) return
              setCopied(true)
              setTimeout(() => setCopied(false), 1500)
            })
          }
          className="flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-foreground/5 hover:text-foreground"
        >
          {copied ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}
        </button>
        {secret && (
          <button type="button" title={shown ? "Masquer" : "Afficher"} onClick={() => setShown(!shown)} className="flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-foreground/5 hover:text-foreground">
            {shown ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
          </button>
        )}
      </div>
    </div>
  )
}
