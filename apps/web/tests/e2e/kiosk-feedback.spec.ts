import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'

// Presentation fixtures only; the real camera/API path is verified separately.
test.use({ launchOptions: { args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'] } })

test('kiosk shows a clear nonblocking confirmation and replaces it for the next pupil', async ({ page }) => {
  let scans = 0
  let pupil = 'A'
  const headers = {
    'Access-Control-Allow-Origin': 'http://localhost:3007',
    'Access-Control-Allow-Credentials': 'true',
    'Access-Control-Allow-Headers': 'Content-Type, X-CSRF-Token',
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
  }
  const now = Date.now()
  await page.route('http://localhost:8087/api/v1/kiosk/**', async route => {
    if (route.request().method() === 'OPTIONS') return route.fulfill({ status: 204, headers })
    if (route.request().url().endsWith('/recognize')) {
      scans++
      return route.fulfill({ headers, json: { data: {
        decision: 'accepted', recognition_enabled: true, engine_status: 'CONFIRMED',
        faces: [{ box: { x: 0.3, y: 0.2, width: 0.3, height: 0.5 }, student_id: pupil, label: `Siswa Uji ${pupil}`, similarity: 0.91 }],
        attendance: { outcome: scans === 1 ? 'recorded' : 'already_recorded', mode: 'check_in', record: { id: pupil, student_id: pupil, status: 'present', check_in_at: new Date(now).toISOString(), check_out_at: null } },
      } } })
    }
    return route.fulfill({ headers, json: { data: {
      device_name: 'Kamera uji tampilan', class_name: 'Kelas uji', session_name: 'Pratinjau absensi',
      timezone: 'Asia/Jakarta', server_time: new Date().toISOString(), csrf_token: 'test-only', recognition_enabled: true,
      expires_at: new Date(now + 600000).toISOString(), starts_at: new Date(now - 60000).toISOString(),
      late_after: new Date(now + 60000).toISOString(), checkout_at: null, closes_at: new Date(now + 3600000).toISOString(),
    } } })
  })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/absensi')
  await page.getByRole('button', { name: 'Aktifkan kamera', exact: true }).click()
  const confirmation = page.getByRole('region', { name: 'Konfirmasi absensi' })
  await expect(confirmation.getByRole('heading', { name: 'Absen masuk berhasil', exact: true })).toBeVisible()
  await expect(confirmation.getByText('Siswa Uji A', { exact: true })).toBeVisible()
  await expect(confirmation.getByText('Selesai. Silakan beri tempat untuk siswa berikutnya.', { exact: true })).toBeVisible()
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(1)
  await page.screenshot({ path: resolve('../../.tools/kiosk-feedback/notification-desktop.png'), fullPage: true, animations: 'disabled' })
  await expect.poll(() => scans).toBeGreaterThan(3)
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(1)
  pupil = 'B'
  await expect(confirmation.getByText('Siswa Uji B', { exact: true })).toBeVisible()
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(1)
  await expect(page.locator('[data-sonner-toast]')).toContainText('Siswa Uji B')
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('button', { name: 'Hentikan kamera', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true)
  const toastBounds=await page.locator('[data-sonner-toast]').boundingBox()
  expect(toastBounds).not.toBeNull()
  expect(toastBounds!.x).toBeGreaterThanOrEqual(0)
  expect(toastBounds!.x+toastBounds!.width).toBeLessThanOrEqual(390)
  await page.screenshot({ path: resolve('../../.tools/kiosk-feedback/notification-mobile.png'), fullPage: true, animations: 'disabled' })
  await page.getByRole('button', { name: 'Hentikan kamera', exact: true }).click()
})
