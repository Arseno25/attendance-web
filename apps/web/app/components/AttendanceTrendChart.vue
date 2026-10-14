<script setup lang="ts">
import { VisAxis, VisGroupedBar, VisXYContainer } from '@unovis/vue'
import {
  ChartContainer,
  ChartCrosshair,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  componentToString,
} from '~/components/ui/chart'
import type { ChartConfig } from '~/components/ui/chart'
import type { AttendanceDay, ReportFilters } from '~/types/reports'

const props = defineProps<{ days: AttendanceDay[]; filters: ReportFilters }>()
const chartConfig = {
  present: { label: 'Tepat waktu', color: 'var(--success-foreground)' },
  late: { label: 'Terlambat', color: 'var(--warning-foreground)' },
  not_recorded: { label: 'Belum tercatat', color: 'var(--muted-foreground)' },
} satisfies ChartConfig
const chartData = computed(() => props.days.map((day, index) => ({ ...day, index })))
type Data = AttendanceDay & { index: number }
const x = (day: Data) => day.index
const y = [(day: Data) => day.present, (day: Data) => day.late, (day: Data) => day.not_recorded]
const colors = Object.values(chartConfig).map((item) => item.color)
const maximum = computed(() =>
  Math.max(1, ...props.days.flatMap((day) => [day.present, day.late, day.not_recorded])),
)
const yTicks = computed(() => {
  const step = Math.max(1, Math.ceil(maximum.value / 4))
  return Array.from({ length: Math.floor(maximum.value / step) + 1 }, (_, index) => index * step)
})
function dayLabel(index: number, full = false) {
  const day = props.days[Math.round(index)]
  if (!day) return ''
  return new Intl.DateTimeFormat('id-ID', {
    day: 'numeric',
    month: full ? 'long' : 'short',
    timeZone: 'UTC',
  }).format(new Date(`${day.date}T00:00:00Z`))
}
</script>

<template>
  <Card class="min-w-0">
    <CardHeader>
      <CardTitle>Kehadiran tujuh hari</CardTitle>
      <CardDescription>
        {{ days[0]?.date }} – {{ filters.to }} · jumlah siswa per sesi.
      </CardDescription>
      <CardAction>
        <Button
          as-child
          variant="outline"
          size="sm"
        >
          <NuxtLink
            :to="{
              path: '/admin/reports',
              query: { ...filters, from: days[0]?.date ?? filters.from },
            }"
          >
            Lihat laporan
          </NuxtLink>
        </Button>
      </CardAction>
    </CardHeader>
    <CardContent class="min-w-0">
      <ClientOnly>
        <ChartContainer
          :config="chartConfig"
          class="h-[320px] w-full aspect-auto"
        >
          <VisXYContainer
            :data="chartData"
            :y-domain="[0, maximum]"
            role="img"
            aria-label="Grafik batang kehadiran tujuh hari"
          >
            <VisGroupedBar
              :x="x"
              :y="y"
              :color="colors"
              :rounded-corners="4"
              :bar-padding="0.15"
              :group-padding="0.15"
            />
            <VisAxis
              type="x"
              :tick-line="false"
              :domain-line="false"
              :grid-line="false"
              :tick-values="chartData.map((day) => day.index)"
              :tick-format="(index: number) => dayLabel(index)"
              :tick-text-width="40"
            />
            <VisAxis
              type="y"
              :tick-line="false"
              :domain-line="false"
              :tick-values="yTicks"
              :tick-format="(value: number) => String(value)"
            />
            <ChartTooltip />
            <ChartCrosshair
              :x="x"
              :y="y"
              :template="
                componentToString(chartConfig, ChartTooltipContent, {
                  labelFormatter: (index) => dayLabel(Number(index), true),
                })
              "
              color="#0000"
            />
          </VisXYContainer>
          <ChartLegendContent class="flex-wrap" />
        </ChartContainer>
        <template #fallback><Skeleton class="h-[320px] w-full" /></template>
      </ClientOnly>
      <p class="sr-only">
        <template
          v-for="day in days"
          :key="day.date"
        >
          {{ day.date }}: {{ day.present }} tepat waktu, {{ day.late }} terlambat,
          {{ day.not_recorded }} belum tercatat.
        </template>
      </p>
    </CardContent>
    <CardFooter class="text-sm text-muted-foreground">
      Hari tanpa sesi absensi tidak dihitung sebagai alfa. Lihat rincian dan unduh laporan di
      Laporan kehadiran.
    </CardFooter>
  </Card>
</template>
