<script setup lang="ts">
import { School } from '@lucide/vue'
import LoginForm from '~/components/login-04/components/LoginForm.vue'
const { branding, load } = useSchoolBranding()
const config = useRuntimeConfig()
useHead(() => ({ title: `Masuk · ${branding.value.app_name}` }))
await load()
</script>

<template>
  <main class="relative flex min-h-svh flex-col items-center justify-center bg-muted p-6 md:p-10">
    <div class="w-full max-w-sm md:max-w-4xl">
      <NuxtLink
        to="/"
        class="mb-6 flex items-center justify-center gap-2 font-semibold"
      >
        <img
          v-if="branding.logo_url"
          :src="`${config.public.apiBase}${branding.logo_url}`"
          alt=""
          class="size-9 shrink-0 rounded-lg object-contain"
        />
        <School
          v-else
          class="size-5"
        />
        <span class="grid min-w-0 text-left">
          <span>{{ branding.app_name }}</span>
          <span
            v-if="branding.name"
            class="text-xs font-normal text-muted-foreground"
          >
            {{ branding.name }}
          </span>
        </span>
      </NuxtLink>
      <LoginForm />
    </div>
  </main>
</template>
