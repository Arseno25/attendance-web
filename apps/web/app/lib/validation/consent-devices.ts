import { z } from 'zod'
import { byteLength, localDateTime, reason, requiredText, uuid } from './shared.ts'

export const pairingSchema = z.object({
  pairing_token: z
    .string()
    .trim()
    .regex(/^[0-9a-f]{64}$/, 'Token pairing harus berisi 64 karakter heksadesimal.'),
  session_id: uuid,
})
export const deviceSchema = z.object({
  name: requiredText('Nama perangkat', 120),
  location: requiredText('Lokasi', 120),
})
export const reasonSchema = z.object({ reason })
export const guardianSchema = z.object({
  full_name: requiredText('Nama wali', 120),
  relationship: z.enum(['parent', 'guardian'], { error: 'Pilih hubungan wali yang valid.' }),
  verification_ref: requiredText('Referensi verifikasi', 120),
})
export const consentSchema = z.object({
  guardian_id: uuid,
  policy_version: requiredText('Versi kebijakan', 80),
  evidence_ref: requiredText('Referensi bukti', 120),
  valid_until: localDateTime.refine(
    (value) => new Date(value).getTime() > Date.now(),
    'Masa berlaku harus setelah waktu sekarang.',
  ),
})
export const enrollmentSchema = z.object({
  frames: z
    .array(
      z
        .string()
        .regex(/^\/9j\/[A-Za-z0-9+/]*={0,2}$/, 'Frame harus berupa JPEG base64.')
        .refine((value) => byteLength(value) <= 384 * 1024, 'Frame maksimal 384 KiB.'),
    )
    .min(3, 'Ambil minimal 3 frame.')
    .max(5, 'Maksimal 5 frame.'),
})
