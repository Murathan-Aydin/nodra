import { GitBranchIcon } from "lucide-react"
import { cn } from "@/lib/utils"

export const SOURCE_LABEL: Record<string, string> = { github: "GitHub App", gitlab: "GitLab", gitea: "Gitea / Forgejo" }

const TINT: Record<string, string> = {
  github: "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900",
  gitlab: "bg-orange-500/15 text-orange-600 dark:text-orange-400",
  gitea: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
}

export function SourceIcon({ type, className }: { type: string; className?: string }) {
  return (
    <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-lg", TINT[type] ?? "bg-accent", className)}>
      <GitBranchIcon className="size-5" />
    </span>
  )
}
