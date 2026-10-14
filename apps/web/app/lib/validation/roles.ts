import { z } from 'zod'
import { requiredText } from './shared.ts'

export const roleScope = z.enum(['school', 'assigned_classes'])
export const roleSchema = z.object({
  name: requiredText('Nama peran', 120),
  description: z.string().trim().max(500, 'Deskripsi maksimal 500 karakter.'),
  scope: roleScope,
  permissions: z
    .array(z.string())
    .refine(
      (permissions) => new Set(permissions).size === permissions.length,
      'Hak akses tidak boleh berulang.',
    ),
})
