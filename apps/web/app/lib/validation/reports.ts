import { z } from './zod.ts'
import { byteLength, optionalDate, optionalText, optionalUuid } from './shared.ts'
import { attendanceFilterStatus } from './attendance.ts'

export const reportSchema = z
  .object({
    from: optionalDate,
    to: optionalDate,
    class_id: optionalUuid,
    academic_year_id: optionalUuid,
    student_id: optionalUuid,
    session_id: optionalUuid,
    status: attendanceFilterStatus,
    search: optionalText('Pencarian', 100),
  })
  .superRefine((value, ctx) => {
    if (!value.from && !value.to) return
    if (!value.from)
      ctx.addIssue({ code: 'custom', path: ['from'], message: 'Isi tanggal mulai rentang.' })
    if (!value.to)
      ctx.addIssue({ code: 'custom', path: ['to'], message: 'Isi tanggal akhir rentang.' })
    if (
      value.from &&
      value.to &&
      (value.to < value.from ||
        new Date(value.to).getTime() - new Date(value.from).getTime() > 365 * 86400000)
    )
      ctx.addIssue({
        code: 'custom',
        path: ['to'],
        message: 'Rentang harus berurutan dan maksimal 366 hari.',
      })
  })
export const auditSchema = z
  .object({
    from: optionalDate,
    to: optionalDate,
    actor_id: optionalUuid,
    action: z
      .string()
      .trim()
      .refine((value) => byteLength(value) <= 100, 'Aktivitas maksimal 100 byte.'),
    target_type: z
      .string()
      .trim()
      .refine((value) => byteLength(value) <= 100, 'Jenis target maksimal 100 byte.'),
  })
  .refine((value) => !value.from || !value.to || value.from <= value.to, {
    path: ['to'],
    error: 'Tanggal akhir harus sama atau setelah tanggal mulai.',
  })
