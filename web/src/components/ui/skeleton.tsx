import * as React from "react"
import { cn } from "@/lib/utils"

export function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("animate-pulse rounded-lg bg-white/10 dark:bg-white/5 backdrop-blur-xs", className)} {...props} />
}
