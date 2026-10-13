import type { ReportFilters } from '~/types/reports'
export function useReports() {
  const { request, options, today, dateTime } = useAttendance()
  const auth = useAuth()
  const canExport = computed(() => auth.session.value?.permissions.includes('reports.export') ?? false)
  function defaultFilters(): ReportFilters { return { from: today(), to: today(), class_id: '', academic_year_id: '', student_id: '', session_id: '', status: '', search: '' } }
  async function download(filters: ReportFilters) {
    const content = await request<string>('/reports/attendance', { query: { ...filters, format: 'csv' } })
    const blob = new Blob([content.startsWith('\uFEFF') ? content : `\uFEFF${content}`], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url; link.download = `attendance_${filters.from}_${filters.to}.csv`
    document.body.appendChild(link); link.click(); link.remove()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return { request, options, today, dateTime, canExport, defaultFilters, download }
}
