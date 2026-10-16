<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { UserRound } from '@lucide/vue'
import type { Result } from '~/types/auth'
import { cn } from '~/lib/utils'

const props = defineProps<{
  studentId?: string
  revision?: string | null
  image?: string
  name: string
  class?: HTMLAttributes['class']
}>()
const request = useApi()
const source = ref('')
let pending: AbortController | undefined
watch(
  () => [props.studentId, props.revision, props.image],
  async () => {
    pending?.abort()
    source.value = props.image ? `data:image/jpeg;base64,${props.image}` : ''
    if (props.image || !props.studentId || !props.revision) return
    const controller = new AbortController()
    pending = controller
    try {
      const result = await request<Result<{ image: string }>>(
        `/students/${props.studentId}/face-photo`,
        { signal: controller.signal },
      )
      if (!controller.signal.aborted) source.value = `data:image/jpeg;base64,${result.data.image}`
    } catch {
      // An unavailable portrait must not block face management or attendance.
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => pending?.abort())
</script>

<template>
  <Avatar
    :key="source"
    :class="cn('size-10 rounded-md', props.class)"
  >
    <AvatarImage
      v-if="source"
      :src="source"
      :alt="`Foto pendaftaran ${name}`"
      class="object-cover"
    />
    <AvatarFallback class="rounded-[inherit]">
      <UserRound
        class="size-1/2 text-muted-foreground"
        aria-hidden="true"
      />
      <span class="sr-only">Foto pendaftaran {{ name }} belum tersedia</span>
    </AvatarFallback>
  </Avatar>
</template>
