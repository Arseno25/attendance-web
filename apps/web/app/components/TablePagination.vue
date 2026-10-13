<script setup lang="ts">
import { ChevronLeft, ChevronRight } from '@lucide/vue'
defineProps<{ page: number, total: number, label: string, disabled: boolean }>()
const emit = defineEmits<{ 'update:page': [page: number] }>()
</script>

<template>
  <div class="flex flex-wrap items-center justify-between gap-3 text-sm">
    <p class="text-muted-foreground">{{ total }} {{ label }} · halaman {{ page }} dari {{ Math.max(1, Math.ceil(total / 20)) }}</p>
    <Pagination class="mx-0 w-auto justify-end" :page="page" :total="total" :items-per-page="20" :disabled="disabled" :aria-label="`Halaman ${label}`" @update:page="emit('update:page', $event)">
      <PaginationContent>
        <PaginationPrevious :disabled="disabled || page <= 1" aria-label="Halaman sebelumnya"><ChevronLeft class="size-4" /><span class="hidden sm:inline">Sebelumnya</span></PaginationPrevious>
        <PaginationNext :disabled="disabled || page * 20 >= total" aria-label="Halaman berikutnya"><span class="hidden sm:inline">Berikutnya</span><ChevronRight class="size-4" /></PaginationNext>
      </PaginationContent>
    </Pagination>
  </div>
</template>
