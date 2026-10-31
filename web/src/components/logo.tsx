import { cn } from "@/lib/utils"

export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={cn("size-7 shrink-0 drop-shadow-sm", className)} aria-hidden="true">
      <defs>
        <linearGradient id="nodra-logo-grad" x1="0" y1="0" x2="0" y2="32" gradientUnits="userSpaceOnUse">
          <stop stopColor="#0a84ff" />
          <stop offset="1" stopColor="#0062cc" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="8" fill="url(#nodra-logo-grad)" />
      <rect width="32" height="32" rx="8" fill="none" stroke="rgba(255,255,255,0.25)" strokeWidth="1" />
      <path d="M9 23V9h2.6l7.8 9.6V9H22v14h-2.6l-7.8-9.6V23z" fill="#ffffff" />
    </svg>
  )
}
