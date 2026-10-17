<script setup lang="ts">
import { ClipboardList, Clock, CircleCheck, UserRound } from '@lucide/vue'
import type { AttendanceTotals } from '~/types/reports'
const props = defineProps<{ totals: AttendanceTotals }>()
const cards = computed(() => [
  {
    label: 'Peserta sesi',
    value: props.totals.roster_total,
    note: `${props.totals.students_total} siswa unik · ${props.totals.sessions_total} sesi`,
    icon: ClipboardList,
  },
  {
    label: 'Sudah hadir',
    value: props.totals.attended,
    note: `${props.totals.present} tepat waktu · termasuk terlambat`,
    icon: CircleCheck,
  },
  {
    label: 'Terlambat',
    value: props.totals.late,
    note: 'Termasuk dalam jumlah sudah hadir',
    icon: Clock,
  },
  {
    label: 'Belum tercatat',
    value: props.totals.not_recorded,
    note: 'Belum dianggap alfa',
    icon: UserRound,
  },
])
</script>
<template>
  <div class="grid gap-4 @xl/main:grid-cols-2 @5xl/main:grid-cols-4">
    <Card
      v-for="card in cards"
      :key="card.label"
    >
      <CardHeader>
        <CardDescription>{{ card.label }}</CardDescription>
        <CardTitle class="text-3xl font-semibold tabular-nums">{{ card.value }}</CardTitle>
        <CardAction>
          <component
            :is="card.icon"
            class="size-5 text-muted-foreground"
          />
        </CardAction>
      </CardHeader>
      <CardFooter class="text-sm text-muted-foreground">{{ card.note }}</CardFooter>
    </Card>
  </div>
  <dl class="flex flex-wrap gap-x-6 gap-y-2 text-sm">
    <div class="flex gap-2">
      <dt class="text-muted-foreground">Izin</dt>
      <dd class="font-medium tabular-nums">{{ totals.excused }}</dd>
    </div>
    <div class="flex gap-2">
      <dt class="text-muted-foreground">Sakit</dt>
      <dd class="font-medium tabular-nums">{{ totals.sick }}</dd>
    </div>
    <div class="flex gap-2">
      <dt class="text-muted-foreground">Alfa</dt>
      <dd class="font-medium tabular-nums">{{ totals.absent }}</dd>
    </div>
    <div class="flex gap-2">
      <dt class="text-muted-foreground">Perlu ditinjau</dt>
      <dd class="font-medium tabular-nums">{{ totals.pending_review }}</dd>
    </div>
    <div class="flex gap-2">
      <dt class="text-muted-foreground">Rasio hadir / daftar siswa</dt>
      <dd class="font-medium tabular-nums">{{ totals.attendance_rate }}%</dd>
    </div>
  </dl>
</template>
