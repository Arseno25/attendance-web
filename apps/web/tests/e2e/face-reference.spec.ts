import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'

test.use({
  launchOptions: { args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'] },
})

// Synthetic portraits exercise UI delivery; encrypted storage and scope checks use real DB tests.
test('face management shows one reference in the table/modal and handles legacy registrations', async ({
  page,
}) => {
  const portrait = await page.evaluate(() => {
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = 192
    return canvas.toDataURL('image/jpeg').split(',')[1]
  })
  const student = {
    id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    full_name: 'Siswa Referensi',
    nis: 'TEST-01',
    status: 'active',
    class_name: 'Kelas uji',
    reference_photo_version: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb' as string | null,
  }
  let photoCalls = 0
  await page.goto('/login')
  await page.getByLabel('Email', { exact: true }).fill('browser-admin@example.test')
  await page.getByLabel('Password', { exact: true }).fill('BrowserTest1!')
  await page.getByRole('button', { name: 'Masuk', exact: true }).click()
  await expect(page).toHaveURL(/\/admin$/)
  await page.route('**/api/v1/biometrics/students?*', (route) =>
    route.fulfill({ json: { data: [student], pagination: { page: 1, page_size: 20, total: 1 } } }),
  )
  await page.route(`**/api/v1/students/${student.id}/face-status`, (route) =>
    route.fulfill({
      json: {
        data: {
          enrolled: true,
          enrollment_allowed: true,
          pipeline_ready: true,
          recognition_enabled: true,
          template: {
            id: 'template',
            reference_photo_version: student.reference_photo_version,
            frame_count: 3,
            status: 'provisional',
          },
        },
      },
    }),
  )
  await page.route(`**/api/v1/students/${student.id}/face-photo`, (route) => {
    photoCalls++
    return route.fulfill({ json: { data: { image: portrait } } })
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/admin/biometrics')
  await expect(page.getByRole('button', { name: 'Muat ulang', exact: true })).toHaveCount(0)
  const table = page.getByRole('table', { name: 'Siswa untuk pengelolaan wajah' })
  await expect(
    table.getByRole('img', { name: 'Foto pendaftaran Siswa Referensi', exact: true }),
  ).toHaveCount(1)
  await page.getByRole('button', { name: 'Kelola wajah: Siswa Referensi', exact: true }).click()
  const dialog = page.getByRole('dialog')
  const settledDialog = () =>
    expect
      .poll(() =>
        dialog.evaluate(
          (element) =>
            element.getAnimations().filter((animation) => animation.playState === 'running').length,
        ),
      )
      .toBe(0)

  await expect(
    dialog.getByRole('img', { name: 'Foto pendaftaran Siswa Referensi', exact: true }),
  ).toHaveCount(1)
  await expect(dialog.getByText('Foto referensi pendaftaran', { exact: true })).toBeVisible()
  await expect(
    dialog.getByText('Ditampilkan saat wajah cocok pada absensi.', { exact: true }),
  ).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Muat ulang status' })).toHaveCount(0)
  await expect(dialog.getByRole('button', { name: 'Ambil foto', exact: true })).toHaveCount(0)
  await settledDialog()
  await page.screenshot({
    path: resolve('../../.tools/face-reference/enrollment-clean.png'),
    fullPage: true,
    animations: 'disabled',
  })
  await dialog.getByRole('button', { name: 'Opsi pendaftaran wajah' }).click()
  await expect(page.getByRole('menuitem', { name: 'Hapus data wajah' })).toBeVisible()
  await page.getByRole('menuitem', { name: 'Hapus data wajah' }).focus()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menu')).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  student.reference_photo_version = null
  await page.reload()
  await expect(table.getByRole('img')).toHaveCount(0)
  const requests = photoCalls
  await page.getByRole('button', { name: 'Kelola wajah: Siswa Referensi', exact: true }).click()
  await expect(
    dialog.getByText('Foto belum tersedia. Perbarui pendaftaran untuk menambahkan foto.', {
      exact: true,
    }),
  ).toBeVisible()
  await expect(dialog.getByRole('img')).toHaveCount(0)
  expect(photoCalls).toBe(requests)
  await settledDialog()
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 600 })
    await expect(dialog).toHaveCSS('max-height', '568px')
    const bounds = await dialog.boundingBox()
    const saveBounds = await dialog
      .getByRole('button', { name: 'Simpan wajah', exact: true })
      .boundingBox()
    expect(saveBounds!.y + saveBounds!.height).toBeLessThanOrEqual(600)
    expect(bounds!.x).toBeGreaterThanOrEqual(0)
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width)
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true)
  }
  await page.route(`**/api/v1/students/${student.id}/face-preview`, (route) =>
    route.fulfill({
      json: {
        data: {
          quality: 'ready',
          face_count: 1,
          faces: [{ box: { x: 0.3, y: 0.2, width: 0.3, height: 0.5 } }],
          message: 'Wajah siap. Ambil foto.',
        },
      },
    }),
  )
  let savedFrames: string[] = []
  await page.route(`**/api/v1/students/${student.id}/enrollments`, (route) => {
    savedFrames = route.request().postDataJSON().frames
    return route.fulfill({ json: { data: {} } })
  })
  await dialog.getByRole('button', { name: 'Aktifkan kamera', exact: true }).click()
  const capture = dialog.getByRole('button', { name: 'Ambil foto', exact: true })
  await expect(capture).toBeEnabled()
  await capture.click()
  await expect(dialog.getByRole('button', { name: 'Simpan wajah', exact: true })).toBeDisabled()
  await dialog.getByRole('button', { name: 'Ulangi foto', exact: true }).click()
  await expect(dialog.getByRole('img', { name: 'Foto 1', exact: true })).toHaveCount(0)
  for (let index = 0; index < 3; index++) {
    await expect(capture).toBeEnabled()
    await capture.click()
  }
  await expect(dialog.getByRole('button', { name: 'Simpan wajah', exact: true })).toBeEnabled()
  await dialog.getByRole('button', { name: 'Simpan wajah', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  expect(savedFrames).toHaveLength(3)
})
