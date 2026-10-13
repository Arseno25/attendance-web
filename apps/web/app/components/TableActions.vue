<script setup lang="ts">
import { EllipsisVertical } from '@lucide/vue'

defineProps<{ label: string, disabled?: boolean }>()
</script>

<template>
  <DropdownMenu :modal="false">
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" size="icon" class="size-8" :disabled="disabled">
        <EllipsisVertical />
        <span class="sr-only">Tindakan untuk {{ label }}</span>
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="min-w-40">
      <slot />
    </DropdownMenuContent>
  </DropdownMenu>
</template>
