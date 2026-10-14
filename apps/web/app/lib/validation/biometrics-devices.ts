import { z } from 'zod'
import { byteLength, reason, requiredText, uuid } from './shared.ts'

export const pairingSchema = z.object({
  pairing_token: z
    .string()
    .trim()
    .regex(/^[0-9a-f]{64}$/, 'Kode penghubung harus berisi 64 karakter heksadesimal.'),
  session_id: uuid,
})
export const deviceSchema = z.object({
  name: requiredText('Nama perangkat', 120),
  location: requiredText('Lokasi', 120),
})
export const reasonSchema = z.object({ reason })
export const enrollmentSchema = z.object({
  frames: z
    .array(
      z
        .string()
        .regex(
          /^\/9j\/[A-Za-z0-9+/]*={0,2}$/,
          'Foto belum dapat diproses. Ambil ulang dari kamera.',
        )
        .refine(
          (value) => byteLength(value) <= 384 * 1024,
          'Foto terlalu besar. Ambil ulang dari kamera.',
        ),
    )
    .min(3, 'Ambil minimal 3 foto.')
    .max(5, 'Maksimal 5 foto.'),
})
