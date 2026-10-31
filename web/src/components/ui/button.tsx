import * as React from "react"
import { Slot } from "radix-ui"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium transition-all outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 disabled:pointer-events-none disabled:opacity-50 active:scale-[0.98] [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 [&_svg]:shrink-0 cursor-pointer",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground shadow-sm hover:brightness-105 shadow-[inset_0_1px_0_0_rgba(255,255,255,0.2)]",
        destructive: "bg-destructive text-white shadow-sm hover:brightness-105 shadow-[inset_0_1px_0_0_rgba(255,255,255,0.2)]",
        outline: "border border-white/15 dark:border-white/10 bg-white/10 dark:bg-white/5 backdrop-blur-md shadow-xs hover:bg-white/20 dark:hover:bg-white/10 text-foreground",
        secondary: "bg-white/10 dark:bg-white/8 text-foreground hover:bg-white/15 backdrop-blur-md",
        ghost: "hover:bg-white/10 dark:hover:bg-white/5 text-muted-foreground hover:text-foreground",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-4 py-2",
        sm: "h-8 gap-1.5 rounded-lg px-3 text-xs",
        lg: "h-10 rounded-lg px-6",
        icon: "size-9 rounded-lg",
        "icon-sm": "size-8 rounded-lg",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
)

export interface ButtonProps extends React.ComponentProps<"button">, VariantProps<typeof buttonVariants> {
  asChild?: boolean
  loading?: boolean
}

export function Button({ className, variant, size, asChild = false, loading, disabled, children, ...props }: ButtonProps) {
  const Comp = asChild ? Slot.Root : "button"
  return (
    <Comp
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      disabled={disabled || loading}
      {...props}
    >
      {asChild ? (
        children
      ) : (
        <>
          {loading && <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" />}
          {children}
        </>
      )}
    </Comp>
  )
}

export { buttonVariants }
