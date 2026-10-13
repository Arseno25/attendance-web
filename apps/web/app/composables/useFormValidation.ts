import { nextTick, ref, watch } from 'vue'
import type { ZodType } from 'zod'

export function useFormValidation(schemaGetter: () => ZodType, valuesGetter: () => unknown) {
  const errors = ref<Record<string, string>>({})
  let submitted = false
  function check() {
    const result = schemaGetter().safeParse(valuesGetter(), {
      error: () => 'Masukkan nilai yang valid.',
    })
    const messages: Record<string, string> = {}
    if (!result.success)
      for (const issue of result.error.issues) {
        const key = String(issue.path[0] ?? 'form')
        messages[key] ??= issue.message
      }
    errors.value = messages
    return result.success
  }
  function validate() {
    submitted = true
    const valid = check()
    if (!valid)
      void nextTick(() => {
        if (typeof document !== 'undefined') {
          const fields = document.querySelectorAll<HTMLElement>(
            '[aria-invalid="true"]:not([disabled])',
          )
          Array.from(fields)
            .find(
              (field) =>
                !field.closest('[inert], [aria-hidden="true"]') && field.getClientRects().length,
            )
            ?.focus()
        }
      })
    return valid
  }
  function clear() {
    submitted = false
    errors.value = {}
  }
  watch(
    valuesGetter,
    () => {
      if (submitted) check()
    },
    { deep: true },
  )
  return { errors, validate, clear }
}
