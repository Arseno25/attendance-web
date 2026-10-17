import { z } from './zod.ts'
import {
  byteLength,
  date,
  optionalText,
  optionalUuid,
  reason,
  requiredText,
  uuid,
} from './shared.ts'

export const academicYearSchema = z
  .object({
    name: requiredText('Nama tahun ajaran', 120),
    starts_on: date,
    ends_on: date,
    is_active: z.boolean(),
  })
  .refine((value) => value.starts_on <= value.ends_on, {
    path: ['ends_on'],
    error: 'Tanggal akhir harus sama atau setelah tanggal mulai.',
  })
export const classSchema = z.object({
  name: requiredText('Nama kelas', 120),
  code: requiredText('Kode kelas', 30),
  level: optionalText('Tingkat', 30),
  is_active: z.boolean(),
})
export const newClassSchema = classSchema.extend({ academic_year_id: uuid })
export const assignmentSchema = z.object({ user_id: uuid, class_id: uuid })
export const studentStatus = z.enum(['active', 'inactive', 'graduated', 'transferred'], {
  error: 'Pilih status siswa yang valid.',
})
export const studentSchema = z.object({
  nis: requiredText('NIS', 30),
  nisn: optionalText('NISN', 32),
  full_name: requiredText('Nama lengkap', 120),
  status: studentStatus,
})
export const newStudentSchema = studentSchema.extend({ class_id: uuid })
export const placementSchema = z.object({ class_id: uuid, reason })
export const searchSchema = z.object({
  search: z
    .string()
    .trim()
    .refine((value) => byteLength(value) <= 100, 'Pencarian maksimal 100 byte.'),
})
export const enrolledFilter = z
  .union([
    z.literal(''),
    z.enum(['true', 'false'], {
      error: 'Pilih status wajah yang valid.',
    }),
  ])
  .optional()
export const studentFilterSchema = searchSchema.extend({
  class_id: optionalUuid,
  status: z.union([z.literal(''), studentStatus]),
  enrolled: enrolledFilter,
})
// shortcut: CSV row parsing and the 500-row limit stay on the server, which handles quoted newlines and reports every domain error.
export const importSchema = z.object({
  class_id: uuid,
  csv: z
    .string()
    .refine((value) => !!value.trim(), 'Pilih file CSV terlebih dahulu.')
    .refine((value) => byteLength(value) <= 120 * 1024, 'File CSV maksimal 120 KiB.'),
})
export const importCommitSchema = z.object({
  id: uuid,
  hash: z.string().regex(/^[0-9a-f]{64}$/, 'Preview import tidak valid. Tinjau ulang file.'),
})
