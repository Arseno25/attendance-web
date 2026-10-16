<script setup lang="ts">
import {
  EllipsisVertical,
  Eye,
  Pencil,
  ScanFace,
  Ban,
  Trash2,
  KeyRound,
  Check,
  X,
  ClipboardCheck,
  LogOut,
  FilePenLine,
} from '@lucide/vue'
import type { TableAction } from '../lib/dataTable'

const props = defineProps<{ label: string; actions: TableAction[]; disabled?: boolean }>()
const actions = computed(() => props.actions.filter((action) => !action.hidden))
function select(action: TableAction) {
  if (!props.disabled && !action.disabled) action.onSelect?.()
}
const icons = {
  view: Eye,
  edit: Pencil,
  face: ScanFace,
  revoke: Ban,
  delete: Trash2,
  pair: KeyRound,
  approve: Check,
  reject: X,
  record: ClipboardCheck,
  checkout: LogOut,
  correction: FilePenLine,
}
</script>

<template>
  <TooltipProvider :delay-duration="150">
    <span
      v-if="!actions.length"
      class="text-muted-foreground"
    >
      —
    </span>
    <div
      v-else-if="actions.length <= 2"
      class="flex items-center justify-end gap-1"
    >
      <Tooltip
        v-for="action in actions"
        :key="action.label"
      >
        <TooltipTrigger as-child>
          <Button
            variant="ghost"
            size="icon"
            class="size-8"
            :class="
              action.destructive
                ? 'text-destructive hover:bg-destructive/10 hover:text-destructive'
                : undefined
            "
            :disabled="disabled || action.disabled"
            :aria-label="`${action.label}: ${label}`"
            :as-child="!!action.to && !disabled && !action.disabled"
            @click="select(action)"
          >
            <NuxtLink
              v-if="action.to && !disabled && !action.disabled"
              :to="action.to"
            >
              <component :is="icons[action.icon]" />
            </NuxtLink>
            <component
              :is="icons[action.icon]"
              v-else
            />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ action.label }}</TooltipContent>
      </Tooltip>
    </div>
    <DropdownMenu
      v-else
      :modal="false"
    >
      <DropdownMenuTrigger as-child>
        <Button
          variant="ghost"
          size="icon"
          class="size-8"
          :disabled="disabled"
        >
          <EllipsisVertical />
          <span class="sr-only">Tindakan untuk {{ label }}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        class="min-w-40"
      >
        <DropdownMenuItem
          v-for="action in actions"
          :key="action.label"
          :variant="action.destructive ? 'destructive' : 'default'"
          :disabled="disabled || action.disabled"
          :as-child="!!action.to && !disabled && !action.disabled"
          @select="select(action)"
        >
          <NuxtLink
            v-if="action.to && !disabled && !action.disabled"
            :to="action.to"
          >
            <component :is="icons[action.icon]" />
            {{ action.label }}
          </NuxtLink>
          <template v-else>
            <component :is="icons[action.icon]" />
            {{ action.label }}
          </template>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </TooltipProvider>
</template>
