import { z } from './zod.ts'

export const byteLength = (value: string) => new TextEncoder().encode(value).length
export const requiredText = (label: string, max: number) =>
  z
    .string()
    .trim()
    .min(1, `${label} wajib diisi.`)
    .refine((value) => [...value].length <= max, `${label} maksimal ${max} karakter.`)
export const optionalText = (label: string, max: number) =>
  z
    .string()
    .trim()
    .refine((value) => [...value].length <= max, `${label} maksimal ${max} karakter.`)
export const uuid = z
  .string()
  .regex(
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
    'Pilih data yang valid.',
  )
export const optionalUuid = z.union([z.literal(''), uuid])
export const reason = requiredText('Alasan', 500)

export function validDate(value: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value) || value < '0001-01-01' || value > '9998-12-31')
    return false
  const parsed = new Date(`${value}T00:00:00Z`)
  return Number.isFinite(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value
}
export const date = z.string().refine(validDate, 'Masukkan tanggal yang valid.')
export const optionalDate = z.union([z.literal(''), date])
export const localDateTime = z.string().refine((value) => {
  const [day, time] = value.split('T')
  return (
    !!day &&
    validDate(day) &&
    !!time &&
    /^([01]\d|2[0-3]):[0-5]\d(?::[0-5]\d)?$/.test(time) &&
    Number.isFinite(new Date(value).getTime())
  )
}, 'Masukkan tanggal dan waktu yang valid.')
export const time = z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/, 'Masukkan jam yang valid.')
