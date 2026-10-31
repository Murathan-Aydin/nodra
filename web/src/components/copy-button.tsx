import { CheckIcon, CopyIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"

export async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    toast.error("Copie impossible : sélectionnez le texte manuellement.")
    return false
  }
}

export function CopyButton({ value, label = "Copier" }: { value: string; label?: string }) {
  const [done, setDone] = useState(false)
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      title={label}
      onClick={async () => {
        if (await copyText(value)) {
          setDone(true)
          setTimeout(() => setDone(false), 1500)
        }
      }}
    >
      {done ? <CheckIcon className="text-success" /> : <CopyIcon />}
    </Button>
  )
}
