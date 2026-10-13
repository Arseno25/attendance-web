export interface Schedule {
  id: string
  class_id: string
  class_name: string
  academic_year_name: string
  name: string
  timezone: string
  weekdays: number[]
  checkin_start: string
  checkin_cutoff: string
  grace_minutes: number
  close_time: string
  is_active: boolean
}
export interface AttendanceSession {
  id: string
  name: string
  class_id: string
  class_name: string
  date: string
  timezone: string
  starts_at: string
  late_after: string
  closes_at: string
  status: string
  reconciled_at: string | null
  roster_count: number
  recorded_count: number
}
export interface Participant {
  id: string
  student_id: string
  nis: string
  full_name: string
  status: string
  check_in_at: string | null
  check_out_at: string | null
  reason: string | null
  correction_pending: boolean
}
export interface Correction {
  id: string
  session_id: string
  session_date: string
  student_id: string
  nis: string
  full_name: string
  class_name: string
  requester_name: string
  requested_by: string
  previous_status: string
  new_status: string
  check_in_at: string | null
  check_out_at: string | null
  reason: string
  status: string
  decision_reason: string | null
}
export interface CalendarDay {
  date: string
  is_school_day: boolean | null
  reason: string
  is_override: boolean
}
export const attendanceLabels: Record<string, string> = {
  not_recorded: 'Belum tercatat',
  present: 'Hadir',
  late: 'Terlambat',
  excused: 'Izin',
  sick: 'Sakit',
  absent: 'Alfa',
  pending_review: 'Perlu review',
}
export const sessionLabels: Record<string, string> = {
  scheduled: 'Terjadwal',
  open: 'Terbuka',
  closed: 'Ditutup',
  cancelled: 'Dibatalkan',
}
export const correctionLabels: Record<string, string> = {
  pending: 'Menunggu',
  approved: 'Disetujui',
  rejected: 'Ditolak',
}
