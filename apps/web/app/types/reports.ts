export interface ReportFilters {
  from: string
  to: string
  class_id: string
  academic_year_id: string
  student_id: string
  session_id: string
  status: string
  search: string
}
export interface AttendanceTotals {
  roster_total: number
  students_total: number
  sessions_total: number
  present: number
  late: number
  attended: number
  excused: number
  sick: number
  absent: number
  not_recorded: number
  pending_review: number
  recorded: number
  attendance_rate: number
}
export interface ReportRow {
  session_id: string
  student_id: string
  class_id: string
  class_name: string
  academic_year_id: string
  academic_year_name: string
  session_name: string
  date: string
  timezone: string
  session_status: string
  nis: string
  full_name: string
  status: string
  check_in_at: string | null
  check_out_at: string | null
  reason: string | null
  updated_at: string
}
export interface AttendanceReport {
  data: ReportRow[]
  totals: AttendanceTotals
  daily: AttendanceDay[]
  filters: ReportFilters
  pagination: { page: number; page_size: number; total: number }
  request_id: string
}
export type AttendanceDay = AttendanceTotals & { date: string }

export interface AttendanceDashboard {
  totals: AttendanceTotals
  filters: ReportFilters
  trend: AttendanceDay[]
  recent: ReportRow[]
}
