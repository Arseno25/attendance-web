<script setup lang="ts">
import { toast } from 'vue-sonner'
import { ImagePlus, Trash2, UserRound, School } from '@lucide/vue'
import { imageUploadSchema } from '~/lib/validation/settings'
import type { Result } from '~/types/auth'

const props = defineProps<{
  endpoint: string
  value?: string
  label: string
  logo?: boolean
  disabled?: boolean
  initials?: string
}>()
const emit = defineEmits<{ updated: []; busyChange: [value: boolean] }>()
const request = useApi()
const config = useRuntimeConfig()
const file = ref<File | null>(null)
const preview = ref('')
const saved = ref<string | undefined>(undefined)
const busy = ref(false)
const inputKey = ref(0)
const picker = ref<{ $el: HTMLInputElement } | null>(null)
const { errors, validate, clear } = useFormValidation(
  () => imageUploadSchema,
  () => ({ file: file.value }),
)
const image = computed(
  () =>
    preview.value ||
    ((saved.value ?? props.value) ? `${config.public.apiBase}${saved.value ?? props.value}` : ''),
)
const inputId = useId()
let alive = true
const controller = new AbortController()

function reset() {
  if (preview.value) URL.revokeObjectURL(preview.value)
  preview.value = ''
  file.value = null
  inputKey.value++
  clear()
}
function choose(event: Event) {
  reset()
  file.value = (event.target as HTMLInputElement).files?.[0] ?? null
  if (file.value && validate()) preview.value = URL.createObjectURL(file.value)
}
async function save(remove = false) {
  if (busy.value || props.disabled || (!remove && !validate())) return
  busy.value = true
  emit('busyChange', true)
  try {
    const result = await request<Result<{ image_url: string }>>(props.endpoint, {
      method: remove ? 'DELETE' : 'PUT',
      ...(remove ? {} : { body: file.value! }),
      signal: controller.signal,
    })
    if (!alive) return
    saved.value = result.data.image_url
    reset()
    emit('updated')
    toast.success(remove ? `${props.label} dihapus.` : `${props.label} diperbarui.`)
  } catch (cause) {
    if (alive) toast.error(apiMessage(cause))
  } finally {
    if (alive) {
      busy.value = false
      emit('busyChange', false)
    }
  }
}
onBeforeUnmount(() => {
  alive = false
  controller.abort()
  reset()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center gap-4">
      <Avatar :class="['size-20 shrink-0', logo ? 'rounded-lg' : '']">
        <AvatarImage
          v-if="image"
          :src="image"
          :alt="label"
          :class="logo ? 'object-contain' : 'object-cover'"
        />
        <AvatarFallback :class="logo ? 'rounded-lg' : ''">
          <School
            v-if="logo"
            class="size-7"
          />
          <span
            v-else-if="initials"
            class="text-xl"
          >
            {{ initials }}
          </span>
          <UserRound
            v-else
            class="size-7"
          />
        </AvatarFallback>
      </Avatar>
      <div class="min-w-0">
        <p class="font-medium">{{ label }}</p>
        <p class="mt-1 text-sm text-muted-foreground">
          {{ file ? 'Gambar baru belum disimpan.' : 'Gunakan JPG atau PNG, maksimal 2 MB.' }}
        </p>
      </div>
    </div>
    <Field :data-invalid="!!errors.file">
      <FieldLabel :for="`${inputId}-button`">Gambar baru</FieldLabel>
      <Input
        :id="inputId"
        :key="inputKey"
        ref="picker"
        class="hidden"
        type="file"
        tabindex="-1"
        :aria-label="`Pilih ${label.toLowerCase()}`"
        accept="image/jpeg,image/png"
        :disabled="busy || disabled"
        @change="choose"
      />
      <Button
        :id="`${inputId}-button`"
        type="button"
        variant="outline"
        class="w-fit"
        :disabled="busy || disabled"
        :aria-invalid="!!errors.file"
        :aria-describedby="errors.file ? `${inputId}-error` : undefined"
        @click="picker?.$el.click()"
      >
        <ImagePlus />
        Pilih gambar
      </Button>
      <FieldDescription
        v-if="file"
        class="break-all"
      >
        {{ file.name }}
      </FieldDescription>
      <FieldError
        v-if="errors.file"
        :id="`${inputId}-error`"
      >
        {{ errors.file }}
      </FieldError>
    </Field>
    <div class="flex flex-wrap justify-end gap-2">
      <Button
        v-if="file"
        type="button"
        variant="outline"
        :disabled="busy || disabled"
        @click="reset"
      >
        Batal
      </Button>
      <Button
        v-else-if="saved ?? value"
        type="button"
        variant="outline"
        :disabled="busy || disabled"
        @click="save(true)"
      >
        <Trash2 />
        Hapus {{ logo ? 'logo' : 'foto' }}
      </Button>
      <Button
        v-if="file"
        type="button"
        :disabled="busy || disabled || !!errors.file"
        @click="save()"
      >
        <Spinner v-if="busy" />
        <ImagePlus v-else />
        {{ busy ? 'Menyimpan…' : logo ? 'Simpan logo' : 'Simpan foto' }}
      </Button>
    </div>
  </div>
</template>
