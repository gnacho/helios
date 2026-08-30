import { type ReactNode } from 'react'
import { Inbox, Search, AlertTriangle } from 'lucide-react'

type Variant = 'empty' | 'no-results' | 'error'

interface EmptyStateProps {
  variant: Variant
  title: string
  description?: string
  hint?: string
  cta?: ReactNode
  icon?: ReactNode
}

const ICONS: Record<Variant, ReactNode> = {
  'empty': <Inbox className="w-10 h-10 text-muted-foreground" />,
  'no-results': <Search className="w-10 h-10 text-muted-foreground" />,
  'error': <AlertTriangle className="w-10 h-10 text-destructive" />,
}

export function EmptyState({ variant, title, description, hint, cta, icon }: EmptyStateProps) {
  const role = variant === 'error' ? 'alert' : variant === 'no-results' ? 'status' : undefined

  return (
    <div className="flex flex-col items-center justify-center py-12 px-4 text-center" role={role}>
      <div className="mb-4">{icon ?? ICONS[variant]}</div>
      <h3 className="text-base font-medium mb-1">{title}</h3>
      {description && <p className="text-sm text-muted-foreground max-w-sm">{description}</p>}
      {hint && <p className="text-xs text-muted-foreground mt-1 max-w-sm">{hint}</p>}
      {cta && <div className="mt-4">{cta}</div>}
    </div>
  )
}

export function EmptyHint({ text }: { text: string }) {
  return <p className="text-sm text-muted-foreground text-center py-4">{text}</p>
}
