import * as React from 'react'

import { cn } from '../../lib/utils'

const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<'textarea'>
>(({ className, ...props }, ref) => {
  return (
    <textarea
      ref={ref}
      className={cn(
        'flex min-h-[56px] w-full rounded-xl border border-[var(--border)] bg-[var(--panel-bg)] px-4 py-3 text-sm text-[var(--text)] shadow-sm',
        'placeholder:text-[var(--text-muted)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent-light)]',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    />
  )
})

Textarea.displayName = 'Textarea'

export { Textarea }
