import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'

const buttonVariants = cva(
    "focus-visible:border-ring focus-visible:ring-ring/50 rounded-lg border border-transparent bg-clip-padding text-sm font-medium focus-visible:ring-3 [&_svg:not([class*='size-'])]:size-4 inline-flex items-center justify-center whitespace-nowrap transition-all disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none shrink-0 [&_svg]:shrink-0 outline-none gap-1.5 select-none cursor-pointer",
    {
        variants: {
            variant: {
                default:
                    'bg-primary text-primary-foreground hover:bg-primary/90',
                outline:
                    'border-border bg-background hover:bg-muted hover:text-foreground dark:bg-input/30 dark:border-input dark:hover:bg-input/50',
                secondary:
                    'bg-secondary text-secondary-foreground hover:bg-secondary/80',
                ghost: 'hover:bg-muted hover:text-foreground dark:hover:bg-muted/50',
                destructive:
                    'bg-destructive text-white hover:bg-destructive/90',
                link: 'text-primary underline-offset-4 hover:underline',
            },
            size: {
                default: 'h-9 px-3',
                sm: 'h-8 px-2.5 text-xs',
                lg: 'h-10 px-4',
                icon: 'size-9',
            },
        },
        defaultVariants: {
            variant: 'default',
            size: 'default',
        },
    },
)

function Button({
    className,
    variant,
    size,
    ...props
}: React.ComponentProps<'button'> & VariantProps<typeof buttonVariants>) {
    return (
        <button
            data-slot='button'
            className={cn(buttonVariants({ variant, size, className }))}
            {...props}
        />
    )
}

export { Button, buttonVariants }
