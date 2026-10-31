import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const badgeVariants = cva(
  "inline-flex w-fit shrink-0 items-center justify-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium whitespace-nowrap transition-colors [&>svg]:size-3",
  {
    variants: {
      variant: {
        default: "border-primary/20 bg-primary/15 text-primary",
        secondary: "border-white/10 bg-white/10 text-muted-foreground",
        outline: "border-white/15 bg-transparent text-foreground",
        success: "border-emerald-500/25 bg-emerald-500/15 text-emerald-400 dark:text-emerald-300",
        warning: "border-amber-500/25 bg-amber-500/15 text-amber-500 dark:text-amber-300",
        destructive: "border-red-500/25 bg-red-500/15 text-red-500 dark:text-red-300",
        info: "border-blue-500/25 bg-blue-500/15 text-blue-500 dark:text-blue-300",
      },
    },
    defaultVariants: { variant: "default" },
  },
)

export function Badge({ className, variant, ...props }: React.ComponentProps<"span"> & VariantProps<typeof badgeVariants>) {
  return <span data-slot="badge" className={cn(badgeVariants({ variant }), className)} {...props} />
}
