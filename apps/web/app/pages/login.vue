<script setup lang="ts">
import { School } from '@lucide/vue'
import LoginForm from '~/components/login-04/components/LoginForm.vue'
useHead({ title: 'Masuk · Absensi Sekolah' })
</script>

<template>
  <main class="relative flex min-h-svh flex-col items-center justify-center bg-muted p-6 md:p-10">
    <div class="absolute right-4 top-4"><ModeToggle /></div>
    <div class="w-full max-w-sm md:max-w-4xl">
      <NuxtLink
        to="/"
        class="mb-6 flex items-center justify-center gap-2 font-semibold"
      >
        <School class="size-5" />
        Absensi Sekolah
      </NuxtLink>
      <LoginForm />
    </div>
  </main>
</template>
