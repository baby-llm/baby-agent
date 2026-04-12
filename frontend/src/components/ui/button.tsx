import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'

import { cn } from '../../lib/utils'

type ButtonVariant = 'default' | 'outline' | 'ghost'
type ButtonSize = 'default' | 'sm' | 'icon'

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  asChild?: boolean
  variant?: ButtonVariant
  size?: ButtonSize
}

const variantClasses: Record<ButtonVariant, string> = {
  default:
    'bg-[var(--accent)] text-white shadow-sm hover:brightness-110',
  outline:
    'border border-[var(--border)] bg-[var(--panel-bg)] text-[var(--text)] shadow-sm hover:bg-white/5',
  ghost:
    'text-[var(--text-muted)] hover:bg-white/5 hover:text-[var(--text)]',
}

const sizeClasses: Record<ButtonSize, string> = {
  default: 'h-10 px-4 py-2',
  sm: 'h-9 rounded-lg px-3 text-xs',
  icon: 'h-9 w-9',
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'default', size = 'default', asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button'

    return (
      <Comp
        className={cn(
          'inline-flex items-center justify-center rounded-lg text-sm font-medium transition-colors',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent-light)] focus-visible:ring-offset-0',
          'disabled:pointer-events-none disabled:opacity-50',
          variantClasses[variant],
          sizeClasses[size],
          className,
        )}
        ref={ref}
        {...props}
      />
    )
  },
)
Button.displayName = 'Button'

export { Button }
