<script setup lang="ts">
import { Building2, ShieldCheck, UserRound } from '@lucide/vue'
import { roleLabels } from '~/types/auth'
const auth = useAuth()
</script>

<template>
  <div v-if="auth.session.value" class="grid gap-4 @xl/main:grid-cols-2 @5xl/main:grid-cols-3">
    <Card class="shadow-none">
      <CardHeader><CardDescription>Sekolah</CardDescription><CardTitle class="break-words text-xl">{{ auth.session.value.school.name }}</CardTitle><CardAction><Building2 class="size-5 text-muted-foreground" /></CardAction></CardHeader>
      <CardFooter class="text-sm text-muted-foreground">Zona waktu {{ auth.session.value.school.timezone }}</CardFooter>
    </Card>
    <Card class="shadow-none">
      <CardHeader><CardDescription>Peran Anda</CardDescription><CardTitle class="text-xl">{{ roleLabels[auth.session.value.user.role] }}</CardTitle><CardAction><ShieldCheck class="size-5 text-muted-foreground" /></CardAction></CardHeader>
      <CardFooter class="text-sm text-muted-foreground">{{ auth.session.value.user.role === 'teacher' ? 'Akses berdasarkan penugasan kelas' : 'Akses sesuai izin sekolah' }}</CardFooter>
    </Card>
    <Card class="shadow-none @xl/main:col-span-2 @5xl/main:col-span-1">
      <CardHeader><CardDescription>Akun Anda</CardDescription><CardTitle class="break-words text-xl">{{ auth.session.value.user.full_name }}</CardTitle><CardAction><UserRound class="size-5 text-muted-foreground" /></CardAction></CardHeader>
      <CardFooter class="break-all text-sm text-muted-foreground">{{ auth.session.value.user.email }}</CardFooter>
    </Card>
  </div>
</template>
