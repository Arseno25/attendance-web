<script setup lang="ts">
import { decisionSchema, correctionFilterSchema } from '~/lib/validation/attendance'
import { statusVariant } from '~/lib/status'
import type { Result } from '~/types/auth'
import { attendanceLabels, correctionLabels, type Correction } from '~/types/attendance'
definePageMeta({ layout: 'admin', middleware: ['auth', 'attendance'] })
const { request, dateTime, canApprove } = useAttendance()
const auth = useAuth()
const entries = ref<Correction[]>([])
const page = ref(1)
const total = ref(0)
const status = ref('pending')
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const success = ref('')
const selected = ref<Correction | null>(null)
const decision = ref('approved')
const reason = ref('')
const { errors, validate, clear } = useFormValidation(
  () => decisionSchema,
  () => ({ decision: decision.value, reason: reason.value }),
)
const { errors: filterErrors, validate: validateFilter } = useFormValidation(
  () => correctionFilterSchema,
  () => ({ status: status.value === 'all' ? '' : status.value }),
)
async function load() {
  if (!validateFilter()) return
  loading.value = true
  message.value = ''
  try {
    const result = await request<Result<Correction[]>>('/attendance-corrections', {
      query: {
        status: status.value === 'all' ? '' : status.value,
        page: page.value,
        page_size: 20,
      },
    })
    entries.value = result.data
    total.value = result.pagination?.total ?? 0
  } catch (error) {
    message.value = apiMessage(error)
  } finally {
    loading.value = false
  }
}
function show(item: Correction, value: string) {
  clear()
  selected.value = item
  decision.value = value
  reason.value = ''
  message.value = ''
}
async function save() {
  if (!selected.value || saving.value || !validate()) return
  saving.value = true
  message.value = ''
  try {
    await request(`/attendance-corrections/${selected.value.id}/decision`, {
      method: 'POST',
      body: { decision: decision.value, reason: reason.value },
    })
    selected.value = null
    success.value = 'Keputusan tersimpan beserta histori dan audit.'
    await load()
  } catch (error) {
    message.value = apiMessage(error)
  } finally {
    saving.value = false
  }
}
async function filter() {
  page.value = 1
  await load()
}
async function changePage(value: number) {
  page.value = value
  await load()
}
await load()
</script>
<template>
  <div class="flex min-w-0 flex-col gap-6">
    <PageHeader
      title="Koreksi absensi"
      description="Tinjau alasan dan perubahan yang diminta. Pemohon tidak dapat menyetujui koreksinya sendiri."
    />
    <Alert
      v-if="message && !selected"
      variant="destructive"
    >
      <AlertDescription>{{ message }}</AlertDescription>
    </Alert>
    <Alert
      v-if="success"
      role="status"
    >
      <AlertDescription>{{ success }}</AlertDescription>
    </Alert>
    <Card class="min-w-0">
      <CardHeader class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle>Antrean koreksi</CardTitle>
          <CardDescription class="mt-1.5">{{ total }} permintaan sesuai filter.</CardDescription>
        </div>
        <Field
          class="sm:w-48"
          :data-invalid="!!filterErrors.status"
        >
          <Select
            v-model="status"
            :disabled="loading"
            @update:model-value="filter"
          >
            <SelectTrigger
              id="correction-status-filter"
              :aria-invalid="!!filterErrors.status"
              :aria-describedby="filterErrors.status ? 'correction-status-filter-error' : undefined"
              aria-label="Filter status koreksi"
              class="w-full sm:w-48"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Semua status</SelectItem>
              <SelectItem
                v-for="(label, key) in correctionLabels"
                :key="key"
                :value="key"
              >
                {{ label }}
              </SelectItem>
            </SelectContent>
          </Select>
          <FieldError
            v-if="filterErrors.status"
            id="correction-status-filter-error"
          >
            {{ filterErrors.status }}
          </FieldError>
        </Field>
      </CardHeader>
      <CardContent class="min-w-0">
        <div
          v-if="loading"
          class="space-y-3"
          role="status"
        >
          <span class="sr-only">Memuat koreksi</span>
          <Skeleton
            v-for="n in 4"
            :key="n"
            class="h-10"
          />
        </div>
        <Table
          v-else
          class="min-w-[900px]"
        >
          <TableCaption class="sr-only">Permintaan koreksi absensi</TableCaption>
          <TableHeader class="bg-muted/50">
            <TableRow>
              <TableHead>Siswa / sesi</TableHead>
              <TableHead>Perubahan</TableHead>
              <TableHead>Alasan / pemohon</TableHead>
              <TableHead>Status</TableHead>
              <TableHead class="w-12 text-right">Tindakan</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="item in entries"
              :key="item.id"
            >
              <TableCell class="font-medium">
                {{ item.full_name }}
                <div class="text-xs font-normal text-muted-foreground">
                  {{ item.nis }} · {{ item.class_name }} · {{ item.session_date }}
                </div>
              </TableCell>
              <TableCell>
                {{ attendanceLabels[item.previous_status] }} →
                {{ attendanceLabels[item.new_status] }}
                <div
                  v-if="item.check_in_at"
                  class="text-xs text-muted-foreground"
                >
                  {{ dateTime(item.check_in_at) }}
                </div>
              </TableCell>
              <TableCell class="max-w-64 whitespace-normal">
                {{ item.reason }}
                <div class="mt-1 text-xs text-muted-foreground">
                  {{ item.requester_name }}
                </div>
              </TableCell>
              <TableCell>
                <Badge :variant="statusVariant(item.status)">
                  {{ correctionLabels[item.status] }}
                </Badge>
                <div
                  v-if="item.decision_reason"
                  class="mt-1 max-w-48 whitespace-normal text-xs text-muted-foreground"
                >
                  {{ item.decision_reason }}
                </div>
              </TableCell>
              <TableCell class="text-right">
                <TableActions
                  :label="item.full_name"
                  :disabled="saving"
                >
                  <DropdownMenuItem as-child>
                    <NuxtLink :to="`/admin/attendance/${item.session_id}`">Lihat sesi</NuxtLink>
                  </DropdownMenuItem>
                  <template
                    v-if="
                      canApprove &&
                      item.status === 'pending' &&
                      item.requested_by !== auth.session.value?.user.id
                    "
                  >
                    <DropdownMenuSeparator />
                    <DropdownMenuItem @select="show(item, 'approved')">
                      Setujui koreksi
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      variant="destructive"
                      @select="show(item, 'rejected')"
                    >
                      Tolak koreksi
                    </DropdownMenuItem>
                  </template>
                  <DropdownMenuLabel
                    v-else-if="
                      item.status === 'pending' && item.requested_by === auth.session.value?.user.id
                    "
                    class="font-normal text-muted-foreground"
                  >
                    Menunggu petugas lain
                  </DropdownMenuLabel>
                </TableActions>
              </TableCell>
            </TableRow>
            <TableRow v-if="!entries.length">
              <TableCell colspan="5">
                <Empty>
                  <EmptyHeader>
                    <EmptyTitle>Tidak ada permintaan sesuai filter</EmptyTitle>
                    <EmptyDescription>
                      Ajukan koreksi dari detail sesi absensi bila ada catatan yang perlu
                      diperbaiki.
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
      <CardFooter class="block border-t">
        <TablePagination
          :page="page"
          :total="total"
          label="koreksi"
          :disabled="loading"
          @update:page="changePage"
        />
      </CardFooter>
    </Card>
    <AlertDialog
      :open="!!selected"
      @update:open="
        (value) => {
          if (!value && !saving) selected = null
        }
      "
    >
      <AlertDialogContent
        class="max-h-[calc(100dvh-2rem)] overflow-y-auto"
        @escape-key-down="
          (event) => {
            if (saving) event.preventDefault()
          }
        "
      >
        <AlertDialogHeader>
          <AlertDialogTitle>
            {{ decision === 'approved' ? 'Setujui' : 'Tolak' }} koreksi?
          </AlertDialogTitle>
          <AlertDialogDescription>
            {{ selected?.full_name }}: {{ attendanceLabels[selected?.previous_status ?? ''] }} →
            {{ attendanceLabels[selected?.new_status ?? ''] }}.
            {{
              decision === 'approved'
                ? 'Persetujuan memperbarui catatan dan menyimpan event perubahan.'
                : 'Penolakan mempertahankan catatan yang sudah ada.'
            }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <form
          novalidate
          class="space-y-4"
          @submit.prevent="save"
        >
          <Alert
            v-if="message"
            variant="destructive"
          >
            <AlertDescription>{{ message }}</AlertDescription>
          </Alert>
          <Field :data-invalid="!!errors.reason">
            <FieldLabel for="decision-reason">Alasan keputusan</FieldLabel>
            <Textarea
              id="decision-reason"
              v-model="reason"
              :aria-invalid="!!errors.reason"
              :aria-describedby="errors.reason ? 'decision-reason-error' : undefined"
              required
              maxlength="500"
              :disabled="saving"
            />
            <FieldError
              v-if="errors.reason"
              id="decision-reason-error"
            >
              {{ errors.reason }}
            </FieldError>
          </Field>
          <AlertDialogFooter>
            <AlertDialogCancel as-child>
              <Button
                type="button"
                variant="outline"
                :disabled="saving"
                @click="selected = null"
              >
                Batal
              </Button>
            </AlertDialogCancel>
            <Button
              type="submit"
              :variant="decision === 'rejected' ? 'destructive' : 'default'"
              :disabled="saving"
            >
              <Spinner v-if="saving" />
              {{ decision === 'approved' ? 'Setujui koreksi' : 'Tolak koreksi' }}
            </Button>
          </AlertDialogFooter>
        </form>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
