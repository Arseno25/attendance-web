import { z } from 'zod'
import { requiredText } from './shared.ts'

export const schoolSettingsSchema = z.object({
  name: requiredText('Nama sekolah', 120),
  app_name: requiredText('Nama aplikasi', 80),
  address: z.string().trim().max(300, 'Alamat maksimal 300 karakter.'),
  phone: z
    .string()
    .trim()
    .max(30, 'Nomor telepon maksimal 30 karakter.')
    .regex(/^[+\d() .-]*$/, 'Masukkan nomor telepon yang valid.'),
  email: z.union([z.literal(''), z.email('Masukkan alamat email yang valid.')]),
})
export const imageUploadSchema = z.object({
  file: z
    .file({ error: 'Pilih gambar terlebih dahulu.' })
    .max(2 * 1024 * 1024, 'Ukuran gambar maksimal 2 MB.')
    .mime(['image/jpeg', 'image/png'], 'Gunakan gambar JPG atau PNG.'),
})
