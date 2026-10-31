import { EyeIcon, EyeOffIcon } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { CopyButton } from "@/components/copy-button"
import { cn } from "@/lib/utils"

/** Valeur sensible affichée masquée, avec affichage et copie. */
export function SecretField({ value, className }: { value: string; className?: string }) {
  const [shown, setShown] = useState(false)
  return (
    <div className={cn("flex min-w-0 items-center gap-1 rounded-md border bg-muted/40 py-1 pr-1 pl-3", className)}>
      <code className="min-w-0 flex-1 truncate font-mono text-xs">{shown ? value : "•".repeat(Math.min(24, Math.max(8, value.length)))}</code>
      <Button type="button" variant="ghost" size="icon-sm" onClick={() => setShown((s) => !s)} aria-label={shown ? "Masquer" : "Afficher"}>
        {shown ? <EyeOffIcon /> : <EyeIcon />}
      </Button>
      <CopyButton value={value} />
    </div>
  )
}
