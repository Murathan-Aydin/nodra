import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const alertVariants = cva("relative w-full rounded-lg border px-4 py-3 text-sm [&>svg]:absolute [&>svg]:top-3.5 [&>svg]:left-4 [&>svg]:size-4 [&>svg~*]:pl-7", {
  variants: {
    variant: {
      default: "bg-card text-card-foreground",
      destructive: "border-destructive/30 bg-destructive/5 text-destructive",
      warning: "border-warning/40 bg-warning/10 text-amber-800 dark:text-warning",
      info: "border-primary/25 bg-accent text-accent-foreground",
    },
  },
  defaultVariants: { variant: "default" },
})

export function Alert({ className, variant, ...props }: React.ComponentProps<"div"> & VariantProps<typeof alertVariants>) {
  return <div role="alert" className={cn(alertVariants({ variant }), className)} {...props} />
}

export function AlertTitle({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("mb-1 font-medium leading-none", className)} {...props} />
}

export function AlertDescription({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("text-sm opacity-90 [&_p]:leading-relaxed", className)} {...props} />
}
