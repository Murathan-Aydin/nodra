import { cn } from "@/lib/utils"

export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={cn("size-7 shrink-0", className)} aria-hidden="true">
      <rect width="32" height="32" rx="8" className="fill-primary" />
      <path d="M9 23V9h2.6l7.8 9.6V9H22v14h-2.6l-7.8-9.6V23z" className="fill-primary-foreground" />
    </svg>
  )
}
