<script setup lang="ts">
import 'vue-sonner/style.css'
import { Toaster } from '~/components/ui/sonner'

const colorMode = useColorMode()
</script>

<template>
  <div class="min-h-screen bg-background text-foreground">
    <NuxtRouteAnnouncer />
    <NuxtLayout><NuxtPage /></NuxtLayout>
    <ClientOnly>
      <Toaster
        :theme="colorMode.value === 'dark' ? 'dark' : 'light'"
        position="top-right"
        rich-colors
        close-button
        :duration="6000"
        container-aria-label="Notifikasi"
        :toast-options="{ closeButtonAriaLabel: 'Tutup notifikasi' }"
      />
    </ClientOnly>
  </div>
</template>
