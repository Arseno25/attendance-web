import { z } from 'zod'
import { byteLength, requiredText } from './shared.ts'

const email = z
  .string()
  .trim()
  .toLowerCase()
  .pipe(z.email({ error: 'Masukkan email yang valid.' }))
  .refine((value) => byteLength(value) <= 254, 'Email maksimal 254 byte.')
const currentPassword = z
  .string()
  .min(1, 'Password wajib diisi.')
  .refine((value) => byteLength(value) <= 128, 'Password maksimal 128 byte.')
const newPassword = z
  .string()
  .refine(
    (value) => byteLength(value) >= 12 && byteLength(value) <= 128,
    'Password harus 12–128 byte.',
  )
const roles = ['super_admin', 'admin', 'operator', 'teacher'] as const
const grantIds = [
  'biometrics.enroll',
  'biometrics.revoke',
  'consents.manage',
  'attendance.manual',
  'attendance.approve',
  'reports.export',
] as const
export const loginSchema = z.object({ email, password: currentPassword })
export function userSchema(editing: boolean) {
  return z
    .object({
      full_name: requiredText('Nama', 120),
      email,
      role: z.enum(roles, { error: 'Pilih peran yang valid.' }),
      password: editing ? z.string() : newPassword,
      is_active: z.boolean(),
      grants: z
        .array(z.enum(grantIds, { error: 'Izin tidak valid.' }))
        .refine((values) => new Set(values).size === values.length, 'Izin tidak boleh berulang.'),
    })
    .refine(
      (value) =>
        value.grants.every(
          (grant) =>
            ['admin', 'super_admin'].includes(value.role) ||
            (value.role === 'operator' &&
              ['biometrics.enroll', 'attendance.manual'].includes(grant)) ||
            (value.role === 'teacher' && grant === 'reports.export'),
        ),
      { path: ['grants'], error: 'Izin tidak sesuai dengan peran yang dipilih.' },
    )
}
export function profileSchema(originalEmail: string) {
  return z
    .object({
      full_name: requiredText('Nama lengkap', 120),
      email,
      current_password: z
        .string()
        .refine((value) => byteLength(value) <= 128, 'Password maksimal 128 byte.'),
      new_password: z
        .string()
        .refine(
          (value) => !value || (byteLength(value) >= 12 && byteLength(value) <= 128),
          'Password harus 12–128 byte.',
        ),
      confirm_password: z.string(),
    })
    .superRefine((value, ctx) => {
      if (
        (value.email !== originalEmail.trim().toLowerCase() || value.new_password !== '') &&
        !value.current_password
      )
        ctx.addIssue({
          code: 'custom',
          path: ['current_password'],
          message: 'Masukkan password saat ini untuk mengubah email atau password.',
        })
      if (value.new_password !== value.confirm_password)
        ctx.addIssue({
          code: 'custom',
          path: ['confirm_password'],
          message: 'Konfirmasi password tidak sama.',
        })
    })
}
