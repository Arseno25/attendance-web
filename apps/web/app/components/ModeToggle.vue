<script setup lang="ts">
import { Laptop, Moon, Sun } from '@lucide/vue'
const colorMode = useColorMode()
const modes = [
  { value: 'light', label: 'Terang', icon: Sun },
  { value: 'dark', label: 'Gelap', icon: Moon },
  { value: 'system', label: 'Sistem', icon: Laptop },
]
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button
        variant="outline"
        size="icon"
        aria-label="Pilih tema"
      >
        <Sun class="size-4 dark:hidden" />
        <Moon class="hidden size-4 dark:block" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuLabel>Tampilan</DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuRadioGroup v-model="colorMode.preference">
        <DropdownMenuRadioItem
          v-for="mode in modes"
          :key="mode.value"
          :value="mode.value"
        >
          <component :is="mode.icon" />
          {{ mode.label }}
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
