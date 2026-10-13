<script setup lang="ts">
import { BookOpen, CalendarDays, GraduationCap, Users } from '@lucide/vue'
import type { Result } from '~/types/auth'
const { request } = useMasterData()
const summary = ref<{ students: number, classes: number, teachers: number, active_year: string | null } | null>(null)
const message = ref('')
try { summary.value = (await request<Result<NonNullable<typeof summary.value>>>('/master-data/summary')).data }
catch (error) { message.value = apiMessage(error) }
const cards = computed(() => summary.value ? [
  { label: 'Siswa aktif', value: summary.value.students, note: 'Siswa dalam lingkup akses Anda', icon: Users },
  { label: 'Kelas aktif', value: summary.value.classes, note: 'Pada tahun ajaran aktif', icon: BookOpen },
  { label: 'Guru bertugas', value: summary.value.teachers, note: 'Dengan penugasan yang berlaku', icon: GraduationCap },
  { label: 'Tahun ajaran', value: summary.value.active_year ?? 'Belum aktif', note: 'Periode akademik sekolah', icon: CalendarDays },
] : [])
</script>
<template>
  <Alert v-if="message" variant="destructive"><AlertDescription>{{ message }}</AlertDescription></Alert>
  <div v-else class="grid gap-4 @xl/main:grid-cols-2 @5xl/main:grid-cols-4">
    <Card v-for="card in cards" :key="card.label"><CardHeader><CardDescription>{{ card.label }}</CardDescription><CardTitle class="text-2xl font-semibold tabular-nums">{{ card.value }}</CardTitle><CardAction><component :is="card.icon" class="size-5 text-muted-foreground" /></CardAction></CardHeader><CardFooter class="text-sm text-muted-foreground">{{ card.note }}</CardFooter></Card>
  </div>
</template>
