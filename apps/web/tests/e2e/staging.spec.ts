import { expect, test, type Page } from '@playwright/test'

async function login(page: Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Password', { exact: true }).fill('BrowserTest1!')
  await page.getByRole('button', { name: 'Masuk', exact: true }).click()
  await expect(page).toHaveURL(/\/admin$/)
}

test('login errors remain inline and account photo can be uploaded and deleted cross-origin', async ({
  page,
}) => {
  await page.goto('/login')
  await page.getByRole('button', { name: 'Masuk', exact: true }).click()
  await expect(page.getByText('Email wajib diisi.', { exact: true })).toBeVisible()
  await login(page, 'browser-admin@example.test')
  await page.goto('/admin/account')
  const photo = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAIAAAAlC+aJAAAAe0lEQVR4nO3PQQ0AIBDAsNOIACTg/4kIHg3JkgnoZu3zdcMFDWhBA1rQgBY0oAUNaEEDWtCAFjSgBQ1oQQNa0IAWNKAFDWhBA1rQgBY0oAUNaEEDWtCAFjSgBQ1oQQNa0IAWNKAFDWhBA1rQgBY0oAUNaEEDWtCAFjx2AbYd4QCqIQHmAAAAAElFTkSuQmCC',
    'base64',
  )
  await page
    .getByLabel('Pilih foto profil', { exact: true })
    .setInputFiles({ name: 'profile.png', mimeType: 'image/png', buffer: photo })
  await page.getByRole('button', { name: /Simpan foto/ }).click()
  await expect(page.getByRole('button', { name: 'Hapus foto', exact: true })).toBeVisible()
  const deletion = page.waitForResponse(
    (response) =>
      response.url().includes('/auth/profile/photo') && response.request().method() === 'DELETE',
  )
  await page.getByRole('button', { name: 'Hapus foto', exact: true }).click()
  expect((await deletion).status()).toBe(200)
  await expect(page.getByRole('button', { name: 'Hapus foto', exact: true })).toHaveCount(0)
})

test('student writer sees its class and can create a student without academic write access', async ({
  page,
}) => {
  await login(page, 'students@example.test')
  await expect(page.getByRole('link', { name: 'Siswa', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Data akademik', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: 'Siswa', exact: true }).click()
  await expect(page.getByRole('cell', { name: /^Browser Student A(?:\s|$)/ })).toBeVisible()
  await expect(page.getByRole('cell', { name: /^Browser Student B(?:\s|$)/ })).toHaveCount(0)
  await page.getByRole('button', { name: 'Tambah siswa', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Nama lengkap', { exact: true }).fill('Browser Created Student')
  await dialog.getByLabel('NIS', { exact: true }).fill('BROWSER-003')
  await dialog.getByLabel('Kelas awal', { exact: true }).click()
  await page.getByRole('option', { name: /Browser A/ }).click()
  await dialog.getByRole('button', { name: /Simpan|Tambah siswa/ }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByRole('cell', { name: /^Browser Created Student(?:\s|$)/ })).toBeVisible()
})

test('report-only custom role can view and download its class report', async ({ page }) => {
  await login(page, 'reports@example.test')
  await page.getByRole('link', { name: 'Laporan kehadiran', exact: true }).click()
  await expect(page.getByRole('link', { name: /^Lihat sesi:/ })).toHaveCount(0)
  await expect(page.getByRole('cell', { name: /^Browser Student A(?:\s|$)/ })).toBeVisible()
  await expect(page.getByRole('cell', { name: /^Browser Student B(?:\s|$)/ })).toHaveCount(0)
  const exportResponse = page.waitForResponse(
    (response) =>
      response.url().includes('/reports/attendance') && response.url().includes('format=csv'),
  )
  await page.getByRole('button', { name: /Ekspor CSV/ }).click()
  const response = await exportResponse
  expect(response.status()).toBe(200)
  expect(await response.text()).toContain('Browser Student A')
  expect(await response.text()).not.toContain('Browser Student B')
  await page.screenshot({
    path: '../../.tools/staging-remediation/reports-browser.png',
    fullPage: true,
  })
})

test('class approver can decide another users correction without request permission', async ({
  page,
}) => {
  await login(page, 'approver@example.test')
  await page.getByRole('link', { name: 'Koreksi absensi', exact: true }).click()
  await expect(page.getByRole('cell', { name: /^Browser Student A(?:\s|$)/ })).toBeVisible()
  await expect(page.getByRole('cell', { name: /^Browser Student B(?:\s|$)/ })).toHaveCount(0)
  await page.getByRole('button', { name: /^Setujui koreksi: Browser Student A$/ }).click()
  const dialog = page.getByRole('alertdialog')
  await dialog
    .getByLabel('Alasan keputusan', { exact: true })
    .fill('Browser verification of independent approval')
  await dialog.getByRole('button', { name: 'Setujui koreksi', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByRole('cell', { name: /^Browser Student A(?:\s|$)/ })).toHaveCount(0)
})

test('operator records attendance and checkout, preserving them after reload and in reports', async ({
  page,
}) => {
  await login(page, 'browser-admin@example.test')
  await page.goto('/admin/attendance')
  await page.getByRole('link', { name: 'Buka detail: Browser A', exact: true }).click()
  const student = page
    .getByRole('row')
    .filter({ has: page.getByRole('cell', { name: /^Browser Attendance Student/ }) })
  await expect(student.getByText('Belum tercatat', { exact: true })).toBeVisible()
  await student
    .getByRole('button', { name: 'Catat absensi: Browser Attendance Student', exact: true })
    .click()
  let dialog = page.getByRole('dialog')
  await dialog
    .getByLabel('Alasan', { exact: true })
    .fill('Kehadiran siswa diperiksa langsung oleh petugas')
  const recorded = page.waitForResponse(
    (r) => r.url().endsWith('/attendances/manual') && r.request().method() === 'POST',
  )
  await dialog.getByRole('button', { name: 'Simpan', exact: true }).click()
  expect((await recorded).status()).toBe(201)
  await expect(dialog).toHaveCount(0)
  await expect(student.getByText(/^(Hadir|Terlambat)$/)).toBeVisible()
  await student
    .getByRole('button', { name: 'Catat pulang: Browser Attendance Student', exact: true })
    .click()
  dialog = page.getByRole('dialog')
  await dialog
    .getByLabel('Alasan', { exact: true })
    .fill('Waktu pulang siswa diperiksa langsung oleh petugas')
  const checkout = page.waitForResponse(
    (r) => r.url().endsWith('/attendances/check-out') && r.request().method() === 'POST',
  )
  await dialog.getByRole('button', { name: 'Simpan', exact: true }).click()
  expect((await checkout).status()).toBe(201)
  await expect(dialog).toHaveCount(0)
  await page.reload()
  await expect(student.getByRole('cell').filter({ hasText: 'Pulang:' })).toBeVisible()
  await page.goto('/admin/reports')
  const report = page
    .getByRole('row')
    .filter({ has: page.getByRole('cell', { name: /^Browser Attendance Student/ }) })
  await expect(report.getByText(/^(Hadir|Terlambat)$/)).toBeVisible()
})
