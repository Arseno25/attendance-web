<script setup lang="ts" generic="TData extends object">
import { tableFeatures, useTable } from '@tanstack/vue-table'
import type { DataTableColumn } from '../lib/dataTable'

const props = defineProps<{
  data: TData[]
  columns: DataTableColumn[]
  caption: string
  rowKey?: keyof TData
  loading?: boolean
}>()
defineSlots<
  { empty?: () => unknown } & { [name: `cell-${string}`]: (props: { item: TData }) => unknown }
>()
const visibleColumns = computed(() => props.columns.filter((column) => !column.hidden))
// Filtering and pagination belong to the caller/server; never slice an already paginated response.
const table = useTable({
  features: tableFeatures({}),
  data: computed(() => props.data),
  columns: computed(() =>
    visibleColumns.value.map((column) => ({ id: column.id, header: column.label })),
  ),
  getRowId: (row: TData, index: number) =>
    props.rowKey ? String(row[props.rowKey]) : String(index),
})
</script>

<template>
  <Table
    :aria-busy="loading || undefined"
    data-slot="data-table"
  >
    <TableCaption class="sr-only">{{ caption }}</TableCaption>
    <TableHeader>
      <TableRow>
        <TableHead
          v-for="column in visibleColumns"
          :key="column.id"
          :class="column.headClass"
          scope="col"
        >
          {{ column.label }}
        </TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow
        v-for="row in table.getRowModel().rows"
        :key="row.id"
      >
        <TableCell
          v-for="column in visibleColumns"
          :key="column.id"
          :class="column.cellClass"
        >
          <slot
            :name="`cell-${column.id}`"
            :item="row.original"
          />
        </TableCell>
      </TableRow>
      <TableRow v-if="!table.getRowModel().rows.length">
        <TableCell
          :colspan="Math.max(visibleColumns.length, 1)"
          class="h-24 text-center"
        >
          <slot name="empty">{{ loading ? 'Memuat data…' : 'Tidak ada data.' }}</slot>
        </TableCell>
      </TableRow>
    </TableBody>
  </Table>
</template>
