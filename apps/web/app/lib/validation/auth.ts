import { z } from 'zod'
import { byteLength, requiredText } from './shared.ts'

const email = z
  .string()
  .trim()
  .toLowerCase()
  .pipe(z.email({ error: 'Masukkan email yang valid.' }))
  .refine((value) => byteLength(value) <= 254, 'Email maksimal 254 byte.')
const currentPassword = z.string().min(1, 'Password wajib diisi.')
export const passwordRules = [
  {
    label: 'Minimal 8 karakter',
    message: 'Password minimal 8 karakter.',
    test: (value: string) => [...value].length >= 8,
  },
  {
    label: '1 huruf besar',
    message: 'Password harus memiliki huruf besar.',
    test: (value: string) => /\p{Lu}/u.test(value),
  },
  {
    label: '1 huruf kecil',
    message: 'Password harus memiliki huruf kecil.',
    test: (value: string) => /\p{Ll}/u.test(value),
  },
  {
    label: '1 angka',
    message: 'Password harus memiliki angka.',
    test: (value: string) => /[0-9]/.test(value),
  },
  {
    label: '1 simbol',
    message: 'Password harus memiliki simbol.',
    test: (value: string) => /[\p{P}\p{S}]/u.test(value),
  },
] as const
const newPassword = passwordRules.reduce(
  (schema, rule) => schema.refine(rule.test, { error: rule.message }),
  z.string(),
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
      current_password: z.string(),
      new_password: newPassword.or(z.literal('')),
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
