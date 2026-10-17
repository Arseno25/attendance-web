import type { BadgeVariants } from '~/components/ui/badge'

const variants: Record<string, BadgeVariants['variant']> = {
  active: 'success',
  present: 'success',
  open: 'success',
  approved: 'success',
  late: 'warning',
  pending: 'warning',
  pending_review: 'warning',
  scheduled: 'warning',
  expired: 'warning',
  sick: 'info',
  excused: 'info',
  graduated: 'info',
  absent: 'destructive',
  rejected: 'destructive',
  revoked: 'destructive',
  withdrawn: 'destructive',
  cancelled: 'destructive',
}

export function statusVariant(status: string): BadgeVariants['variant'] {
  return variants[status] ?? 'secondary'
}

export function recognitionStatusText(operations: {
  pipeline_ready: boolean
  recognition_enabled: boolean
  operation_mode?: string
}): string {
  if (!operations.pipeline_ready) return 'Layanan wajah sedang disiapkan.'
  if (operations.recognition_enabled) return 'Absensi wajah otomatis sudah aktif.'
  switch (operations.operation_mode) {
    case 'disabled':
      return 'Absensi wajah sedang dinonaktifkan. Gunakan absensi manual.'
    case 'shadow':
      return 'Sistem masih dalam masa uji coba. Absensi wajah otomatis belum aktif.'
    case 'approved':
      return 'Menunggu persetujuan sebelum absensi wajah otomatis aktif.'
    default:
      return 'Absensi wajah otomatis belum aktif. Gunakan absensi manual.'
  }
}
