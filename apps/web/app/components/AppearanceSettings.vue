<script setup lang="ts">
import { Laptop, Moon, Sun } from '@lucide/vue'
import { z } from 'zod'
const colorMode = useColorMode()
const theme = z.enum(['light', 'dark', 'system'])
const modes = [
  { value: 'light', label: 'Terang', icon: Sun },
  { value: 'dark', label: 'Gelap', icon: Moon },
  { value: 'system', label: 'Sistem', icon: Laptop },
]
function selectTheme(value: unknown) {
  const result = theme.safeParse(value)
  if (result.success) colorMode.preference = result.data
}
</script>

<template>
  <Card class="min-w-0">
    <CardHeader>
      <CardTitle>Tampilan</CardTitle>
      <CardDescription>Sesuaikan tema aplikasi dengan kenyamanan Anda.</CardDescription>
    </CardHeader>
    <CardContent>
      <FieldGroup>
        <Field>
          <FieldLabel for="appearance-theme">Tema</FieldLabel>
          <Select
            :model-value="colorMode.preference"
            @update:model-value="selectTheme"
          >
            <SelectTrigger
              id="appearance-theme"
              class="w-full sm:w-64"
              aria-describedby="appearance-theme-help"
            >
              <SelectValue placeholder="Pilih tema" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                v-for="mode in modes"
                :key="mode.value"
                :value="mode.value"
              >
                <component
                  :is="mode.icon"
                  class="size-4"
                />
                {{ mode.label }}
              </SelectItem>
            </SelectContent>
          </Select>
          <FieldDescription id="appearance-theme-help">
            Tema Sistem mengikuti pengaturan perangkat Anda. Pilihan tersimpan otomatis di peramban
            ini.
          </FieldDescription>
        </Field>
      </FieldGroup>
    </CardContent>
  </Card>
</template>
