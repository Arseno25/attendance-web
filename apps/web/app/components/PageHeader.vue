<script setup lang="ts">
defineProps<{ title: string; description?: string }>()
</script>

<template>
  <header class="flex flex-wrap items-start justify-between gap-4">
    <div class="min-w-0 basis-full sm:flex-1 sm:basis-auto">
      <h1 class="text-2xl font-semibold tracking-tight">{{ title }}</h1>
      <p
        v-if="description"
        class="mt-2 max-w-2xl text-sm text-muted-foreground"
      >
        {{ description }}
      </p>
    </div>
    <div
      v-if="$slots.actions"
      class="ml-auto flex shrink-0 flex-wrap justify-end gap-2"
    >
      <slot name="actions" />
    </div>
  </header>
</template>
