import type { BadgeVariants } from '~/components/ui/badge'

const variants: Record<string, BadgeVariants['variant']> = {
  active: 'success', present: 'success', open: 'success', approved: 'success',
  late: 'warning', pending: 'warning', pending_review: 'warning', scheduled: 'warning', expired: 'warning',
  sick: 'info', excused: 'info', graduated: 'info',
  absent: 'destructive', rejected: 'destructive', revoked: 'destructive', withdrawn: 'destructive', cancelled: 'destructive',
}

export function statusVariant(status: string): BadgeVariants['variant'] {
  return variants[status] ?? 'secondary'
}
