<script setup lang="ts">
import { Check, Circle } from '@lucide/vue'
import { passwordRules } from '~/lib/validation/auth'

defineProps<{ value: string }>()
</script>

<template>
  <div class="space-y-2">
    <FieldDescription>Password harus memenuhi:</FieldDescription>
    <ul class="grid gap-2 text-sm sm:grid-cols-2">
      <li
        v-for="rule in passwordRules"
        :key="rule.label"
        class="flex items-center gap-2"
        :class="rule.test(value) ? 'text-success' : 'text-muted-foreground'"
      >
        <Check
          v-if="rule.test(value)"
          class="size-4 shrink-0"
          aria-hidden="true"
        />
        <Circle
          v-else
          class="size-4 shrink-0"
          aria-hidden="true"
        />
        <span>
          {{ rule.label }}
          <span class="sr-only">{{ rule.test(value) ? ': terpenuhi' : ': belum terpenuhi' }}</span>
        </span>
      </li>
    </ul>
  </div>
</template>
