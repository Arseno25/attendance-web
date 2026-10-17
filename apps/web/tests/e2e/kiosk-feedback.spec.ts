import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'

// Presentation fixtures only; the real camera/API path is verified separately.
test.use({
  launchOptions: { args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'] },
})

test('kiosk shows a clear nonblocking confirmation and replaces it for the next pupil', async ({
  page,
}) => {
  let scans = 0
  let pupil = 'A'
  let unknownScans = 0
  const portraits = await page.evaluate(() =>
    ['#0369a1', '#15803d'].map((color) => {
      const canvas = document.createElement('canvas')
      canvas.width = canvas.height = 192
      const context = canvas.getContext('2d')!
      context.fillStyle = color
      context.fillRect(0, 0, 192, 192)
      return canvas.toDataURL('image/jpeg').split(',')[1]
    }),
  )
  const headers = {
    'Access-Control-Allow-Origin': 'http://localhost:3007',
    'Access-Control-Allow-Credentials': 'true',
    'Access-Control-Allow-Headers': 'Content-Type, X-CSRF-Token',
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
  }
  const now = Date.now()
  await page.route('http://localhost:8087/api/v1/kiosk/**', async (route) => {
    if (route.request().method() === 'OPTIONS') return route.fulfill({ status: 204, headers })
    if (route.request().url().endsWith('/capture-challenge'))
      return route.fulfill({ headers, json: { data: { event_id: crypto.randomUUID() } } })
    if (route.request().url().endsWith('/recognize')) {
      scans++
      if (pupil === 'unknown') {
        unknownScans++
        return route.fulfill({
          headers,
          json: {
            data: {
              decision: 'unknown',
              recognition_enabled: false,
              faces: [{ box: { x: 0.3, y: 0.2, width: 0.3, height: 0.5 } }],
            },
          },
        })
      }
      return route.fulfill({
        headers,
        json: {
          data: {
            decision: 'accepted',
            recognition_enabled: true,
            engine_status: 'CONFIRMED',
            reference_photo:
              pupil === 'A' ? portraits[0] : pupil === 'B' ? portraits[1] : undefined,
            faces: [
              {
                box: { x: 0.3, y: 0.2, width: 0.3, height: 0.5 },
                student_id: pupil,
                label: `Siswa Uji ${pupil}`,
                similarity: pupil === 'A' && scans > 1 ? 0.72 : 0.91,
              },
            ],
            attendance: {
              outcome: scans === 1 ? 'recorded' : 'already_recorded',
              mode: 'check_in',
              record: {
                id: pupil,
                student_id: pupil,
                status: 'present',
                check_in_at: new Date(now).toISOString(),
                check_out_at: null,
              },
            },
          },
        },
      })
    }
    return route.fulfill({
      headers,
      json: {
        data: {
          device_name: 'Kamera uji tampilan',
          class_name: 'Kelas uji',
          session_name: 'Pratinjau absensi',
          timezone: 'Asia/Jakarta',
          server_time: new Date().toISOString(),
          csrf_token: 'test-only',
          recognition_enabled: true,
          expires_at: new Date(now + 600000).toISOString(),
          starts_at: new Date(now - 60000).toISOString(),
          late_after: new Date(now + 60000).toISOString(),
          checkout_at: null,
          closes_at: new Date(now + 3600000).toISOString(),
        },
      },
    })
  })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/absensi')
  await page.getByRole('button', { name: 'Aktifkan kamera', exact: true }).click()
  const confirmation = page.getByRole('region', { name: 'Konfirmasi absensi' })
  await expect(
    confirmation.getByRole('heading', { name: 'Absen masuk berhasil', exact: true }),
  ).toBeVisible()
  await expect(confirmation.getByText('Siswa Uji A', { exact: true })).toBeVisible()
  await expect(
    confirmation.getByText('Selesai. Silakan beri tempat untuk siswa berikutnya.', { exact: true }),
  ).toBeVisible()
  await expect(
    confirmation.getByRole('img', { name: 'Foto pendaftaran Siswa Uji A', exact: true }),
  ).toBeVisible()
  await expect(confirmation.getByRole('progressbar')).toHaveCount(0)
  // The public kiosk never shows raw biometric scores or other technical figures.
  await expect(confirmation.getByRole('definition').filter({ hasText: /\d,\d\d/ })).toHaveCount(0)
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(1)
  await page.screenshot({
    path: resolve('../../.tools/kiosk-feedback/notification-desktop.png'),
    fullPage: true,
    animations: 'disabled',
  })
  await expect.poll(() => scans).toBeGreaterThan(3)
  await expect(confirmation.getByRole('definition').filter({ hasText: /\d,\d\d/ })).toHaveCount(0)
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(1)
  pupil = 'B'
  await expect(confirmation.getByText('Siswa Uji B', { exact: true })).toBeVisible()
  await expect(
    confirmation.getByRole('img', { name: 'Foto pendaftaran Siswa Uji B', exact: true }),
  ).toBeVisible()
  await expect(
    confirmation.getByRole('img', { name: 'Foto pendaftaran Siswa Uji A', exact: true }),
  ).toHaveCount(0)
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(1)
  await expect(page.locator('[data-sonner-toast]')).toContainText('Siswa Uji B')
  await expect(confirmation.getByRole('img')).toHaveAttribute(
    'src',
    `data:image/jpeg;base64,${portraits[1]}`,
  )
  pupil = 'unknown'
  await expect.poll(() => unknownScans).toBeGreaterThan(1)
  await expect(confirmation.getByRole('img')).toHaveAttribute(
    'src',
    `data:image/jpeg;base64,${portraits[1]}`,
  )
  await expect(confirmation.getByText('Siswa Uji unknown', { exact: true })).toHaveCount(0)
  pupil = 'C'
  await expect(confirmation.getByText('Siswa Uji C', { exact: true })).toBeVisible()
  await expect(confirmation.getByRole('img')).toHaveCount(0)
  await expect(
    confirmation.getByText('Foto pendaftaran Siswa Uji C belum tersedia', { exact: true }),
  ).toHaveCount(1)

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('button', { name: 'Hentikan kamera', exact: true })).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true)
  const toastBounds = await page.locator('[data-sonner-toast]').boundingBox()
  expect(toastBounds).not.toBeNull()
  expect(toastBounds!.x).toBeGreaterThanOrEqual(0)
  expect(toastBounds!.x + toastBounds!.width).toBeLessThanOrEqual(390)
  await page.screenshot({
    path: resolve('../../.tools/kiosk-feedback/notification-mobile.png'),
    fullPage: true,
    animations: 'disabled',
  })
  await page.getByRole('button', { name: 'Hentikan kamera', exact: true }).click()
})

test('remembered device waits, resumes the next session and recovers network without pairing', async ({
  page,
}) => {
  await page.clock.install()
  let mode: 'waiting' | 'ready' | 'offline' | 'revoked' = 'waiting'
  const headers = {
    'Access-Control-Allow-Origin': 'http://localhost:3007',
    'Access-Control-Allow-Credentials': 'true',
    'Access-Control-Allow-Headers': 'Content-Type, X-CSRF-Token',
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
  }
  await page.route('http://localhost:8087/api/v1/kiosk/**', (route) => {
    if (route.request().method() === 'OPTIONS') return route.fulfill({ status: 204, headers })
    if (mode === 'offline') return route.abort('failed')
    if (mode === 'revoked')
      return route.fulfill({
        status: 401,
        headers,
        json: { error: { code: 'DEVICE_UNAUTHENTICATED' } },
      })
    return route.fulfill({
      headers,
      json: {
        data: {
          device_name: 'Perangkat uji',
          class_name: 'Kelas A',
          timezone: 'Asia/Jakarta',
          session_id: mode === 'ready' ? 'next' : null,
          session_name: mode === 'ready' ? 'Sesi kelas berikutnya' : null,
          session_ready: mode === 'ready',
          connection_status: mode === 'ready' ? 'ready' : 'waiting_session',
          server_time: new Date().toISOString(),
          expires_at: new Date(Date.now() + 30 * 86400000).toISOString(),
          closes_at: new Date(Date.now() + 3600000).toISOString(),
          csrf_token: 'fixture',
          recognition_enabled: true,
        },
      },
    })
  })
  await page.goto('/absensi')
  await expect(
    page.getByRole('heading', { name: 'Perangkat sudah terhubung', exact: true }),
  ).toBeVisible()
  await expect(page.getByRole('button', { name: 'Pasangkan', exact: true })).toHaveCount(0)
  await page.reload()
  await expect(
    page.getByRole('heading', { name: 'Perangkat sudah terhubung', exact: true }),
  ).toBeVisible()
  mode = 'ready'
  await page.clock.fastForward(15000)
  await expect(
    page.getByRole('heading', { name: 'Sesi kelas berikutnya', exact: true }),
  ).toBeVisible()
  await expect(page.getByRole('button', { name: 'Aktifkan kamera', exact: true })).toBeVisible()
  mode = 'offline'
  await page.clock.fastForward(15000)
  await expect(page.getByRole('alert').filter({ hasText: 'Koneksi terganggu' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Pasangkan', exact: true })).toHaveCount(0)
  mode = 'ready'
  await page.clock.fastForward(15000)
  await expect(page.getByRole('alert').filter({ hasText: 'Koneksi terganggu' })).toHaveCount(0)
  mode = 'revoked'
  await page.clock.fastForward(15000)
  await expect(
    page.getByRole('heading', { name: 'Hubungkan perangkat', exact: true }),
  ).toBeVisible()
})
