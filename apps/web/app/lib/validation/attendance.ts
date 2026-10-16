import { z } from './zod.ts'
import {
  byteLength,
  date,
  localDateTime,
  optionalDate,
  optionalUuid,
  reason,
  requiredText,
  time,
  uuid,
} from './shared.ts'

export const attendanceStatus = z.enum(['present', 'late', 'excused', 'sick', 'absent'], {
  error: 'Pilih status kehadiran yang valid.',
})
export const attendanceFilterStatus = z.enum(
  ['', 'not_recorded', 'pending_review', 'present', 'late', 'excused', 'sick', 'absent'],
  { error: 'Pilih status kehadiran yang valid.' },
)
const scheduleFields = z.object({
  name: requiredText('Nama jadwal', 120),
  weekdays: z
    .array(z.number().int().min(1).max(7))
    .min(1, 'Pilih minimal satu hari.')
    .refine((value) => new Set(value).size === value.length, 'Hari aktif tidak boleh berulang.'),
  checkin_start: time,
  checkin_cutoff: time,
  close_time: time,
  checkout_start: z.union([time, z.literal('')]).optional(),
  grace_minutes: z
    .union([z.number(), z.string().min(1, 'Toleransi wajib diisi.')])
    .pipe(
      z.coerce
        .number<string | number>()
        .int('Toleransi harus bilangan bulat.')
        .min(0, 'Toleransi minimal 0 menit.')
        .max(120, 'Toleransi maksimal 120 menit.'),
    ),
  is_active: z.boolean(),
})
const minutes = (value: string) => Number(value.slice(0, 2)) * 60 + Number(value.slice(3, 5))
function scheduleOrder(value: z.infer<typeof scheduleFields>, ctx: z.RefinementCtx) {
  if (
    value.checkout_start &&
    (minutes(value.checkout_start) <= minutes(value.checkin_cutoff) + value.grace_minutes ||
      value.checkout_start >= value.close_time)
  )
    ctx.addIssue({
      code: 'custom',
      path: ['checkout_start'],
      message:
        'Jam pulang harus setelah batas tepat waktu dan toleransi, serta sebelum tutup absensi.',
    })

  if (value.checkin_start >= value.checkin_cutoff)
    ctx.addIssue({
      code: 'custom',
      path: ['checkin_cutoff'],
      message: 'Batas tepat waktu harus setelah jam mulai.',
    })
  if (minutes(value.checkin_cutoff) + value.grace_minutes >= minutes(value.close_time))
    ctx.addIssue({
      code: 'custom',
      path: ['close_time'],
      message: 'Jam tutup harus setelah batas tepat waktu dan toleransi.',
    })
}
export const scheduleSchema = scheduleFields.superRefine(scheduleOrder)
export const newScheduleSchema = scheduleFields
  .extend({ class_id: uuid })
  .superRefine(scheduleOrder)
export const calendarDateSchema = z.object({ date })
export const calendarSchema = calendarDateSchema.extend({ is_school_day: z.boolean(), reason })
export const calendarRangeSchema = z
  .object({ from: date, to: date, is_school_day: z.boolean(), reason })
  .superRefine((value, ctx) => {
    if (
      value.to < value.from ||
      Date.parse(`${value.to}T00:00:00Z`) - Date.parse(`${value.from}T00:00:00Z`) > 365 * 86400000
    )
      ctx.addIssue({
        code: 'custom',
        path: ['to'],
        message: 'Pilih rentang berurutan maksimal 366 hari.',
      })
  })
export const sessionSchema = z.object({ schedule_id: uuid, date })
export const sessionFilterSchema = z.object({ date: optionalDate })
export const rosterFilterSchema = z.object({
  search: z.string().refine((value) => byteLength(value) <= 100, 'Pencarian maksimal 100 byte.'),
  status: attendanceFilterStatus,
})
export const rosterSchema = z.object({ student_id: uuid, reason })
export const checkoutSchema = rosterSchema.extend({ session_id: uuid, event_id: uuid })
export const manualSchema = checkoutSchema.extend({
  status: z.enum(['present', 'excused', 'sick'], { error: 'Pilih Hadir, Izin atau Sakit.' }),
})
export const correctionSchema = rosterSchema
  .extend({ session_id: uuid, status: attendanceStatus, check_in_local: z.string() })
  .superRefine((value, ctx) => {
    if (
      ['present', 'late'].includes(value.status) &&
      !localDateTime.safeParse(value.check_in_local).success
    )
      ctx.addIssue({
        code: 'custom',
        path: ['check_in_local'],
        message: 'Masukkan waktu hadir yang valid dalam zona waktu sekolah.',
      })
  })
export const sessionActionSchema = z.object({
  action: z.enum(['open', 'close', 'cancel', 'reconcile']),
  reason,
})
export const decisionSchema = z.object({
  decision: z.enum(['approved', 'rejected'], { error: 'Pilih keputusan yang valid.' }),
  reason,
})
export const correctionFilterSchema = z.object({
  status: z.enum(['', 'pending', 'approved', 'rejected'], {
    error: 'Pilih status koreksi yang valid.',
  }),
})
export const dashboardSchema = z.object({
  date: optionalDate,
  academic_year_id: optionalUuid,
  class_id: optionalUuid,
})
