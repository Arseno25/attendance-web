import { expect, test, type BrowserContext, type Page } from '@playwright/test'
let adminCookies: Awaited<ReturnType<BrowserContext['cookies']>> | undefined
async function calendar(page: Page) {
  if (adminCookies) await page.context().addCookies(adminCookies)
  else {
    await page.goto('/login')
    await page.getByLabel('Email', { exact: true }).fill('browser-admin@example.test')
    await page.getByLabel('Password', { exact: true }).fill('BrowserTest1!')
    await page.getByRole('button', { name: 'Masuk', exact: true }).click()
    await expect(page).toHaveURL(/\/admin$/)
    adminCookies = await page.context().cookies()
  }
  await page.goto('/admin/schedules')
  await page.getByRole('tab', { name: 'Kalender sekolah', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Kalender sekolah', exact: true })).toBeVisible()
}
const month = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Asia/Jakarta',
  year: 'numeric',
  month: '2-digit',
})
  .format(new Date())
  .slice(0, 7)

test('national and joint leave calendar supports school ranges, reload and restoring automatic holidays', async ({
  page,
}) => {
  await calendar(page)
  await expect(page.getByRole('table', { name: 'Kalender libur bulan ini' })).toContainText(
    'Libur nasional uji',
  )
  await expect(page.getByRole('table')).toContainText('Cuti bersama uji')
  await expect(page.getByRole('button', { name: 'Atur tanggal', exact: true })).toHaveCount(0)
  await expect(
    page.locator(
      `[data-slot="calendar-cell-trigger"][data-value="${month}-25"]:not([data-outside-view])`,
    ),
  ).toHaveAccessibleName(/Libur nasional/)
  await expect(
    page.locator(
      `[data-slot="calendar-cell-trigger"][data-value="${month}-26"]:not([data-outside-view])`,
    ),
  ).toHaveAccessibleName(/Cuti bersama/)

  await page.getByRole('button', { name: 'Tambah libur sekolah', exact: true }).click()
  await page.getByLabel('Tanggal mulai', { exact: true }).fill(`${month}-25`)
  await page.getByLabel('Tanggal selesai', { exact: true }).fill(`${month}-24`)
  await page.getByLabel('Keterangan', { exact: true }).fill('Libur semester uji browser')
  await page.getByRole('button', { name: 'Simpan pengaturan', exact: true }).click()
  await expect(page.getByText('Pilih rentang berurutan maksimal 366 hari.')).toBeVisible()
  await page.getByLabel('Tanggal selesai', { exact: true }).fill(`${month}-26`)
  await page.getByRole('button', { name: 'Simpan pengaturan', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('table')).toContainText('Libur semester uji browser')
  await page.reload()
  await page.getByRole('tab', { name: 'Kalender sekolah', exact: true }).click()
  await expect(page.getByRole('table')).toContainText('Libur semester uji browser')
  await page
    .locator(
      `[data-slot="calendar-cell-trigger"][data-value="${month}-25"]:not([data-outside-view])`,
    )
    .click()
  await page.getByRole('button', { name: 'Gunakan kalender otomatis', exact: true }).click()
  await expect(page.getByRole('alertdialog')).toBeVisible()
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: 'Gunakan kalender otomatis', exact: true })
    .click()
  await expect(page.getByRole('alertdialog')).toHaveCount(0)
  await expect(page.getByRole('table')).toContainText('Libur nasional uji')
})

test('calendar and range dialog remain usable at mobile and desktop widths', async ({ page }) => {
  await calendar(page)
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
      .toBeLessThanOrEqual(width)
    await page.getByRole('button', { name: 'Tambah libur sekolah', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    const box = await dialog.boundingBox()
    expect(box!.x).toBeGreaterThanOrEqual(0)
    expect(box!.x + box!.width).toBeLessThanOrEqual(width)
    await page.keyboard.press('Escape')
    await expect(
      page.getByRole('button', { name: 'Tambah libur sekolah', exact: true }),
    ).toBeFocused()
  }
  await page.screenshot({
    path: '../../.tools/holiday-calendar/calendar-desktop.png',
    fullPage: true,
  })
})

test('an unavailable holiday refresh preserves the school calendar on screen', async ({ page }) => {
  await calendar(page)
  await page.route('**/school/calendar/sync', (route) =>
    route.fulfill({
      status: 503,
      json: {
        error: { code: 'HOLIDAYS_UNAVAILABLE', message: 'Sumber libur sementara tidak tersedia.' },
      },
    }),
  )
  await page.getByRole('button', { name: 'Perbarui libur nasional', exact: true }).click()
  await expect(
    page.getByText('Sumber libur sementara tidak tersedia.', { exact: true }),
  ).toBeVisible()
  await expect(page.getByRole('table')).toContainText('Libur nasional uji')
})

test('changing month rejects a late response from the previous month', async ({ page }) => {
  await calendar(page)
  await expect(page.getByRole('table')).toContainText('Libur nasional uji')
  let release!: () => void
  let started!: () => void
  let finished!: () => void
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  const pending = new Promise<void>((resolve) => {
    started = resolve
  })
  const completed = new Promise<void>((resolve) => {
    finished = resolve
  })
  await page.route('**/school/calendar/month?*', async (route) => {
    const response = await route.fetch()
    if (new URL(route.request().url()).searchParams.get('month') !== month) {
      started()
      await held
      try {
        await route.fulfill({ response })
      } catch {
        /* Browser may have already cancelled this request. */
      } finally {
        finished()
      }
    } else await route.fulfill({ response })
  })
  await page.getByRole('button', { name: 'Bulan berikutnya', exact: true }).click()
  await pending
  await page.getByRole('button', { name: 'Bulan sebelumnya', exact: true }).click()
  await expect(page.getByRole('table')).toContainText('Libur nasional uji')
  release()
  await completed
  await expect(page.getByRole('table')).toContainText('Libur nasional uji')
})

test.describe('current school date', () => {
  test.use({ timezoneId: 'America/Los_Angeles' })
  const year = new Date().getUTCFullYear()
  const selectedDate = '[data-slot="calendar-cell-trigger"][data-selected]'

  test('opens on the school date and returns to today after browsing another month', async ({
    page,
  }) => {
    await page.clock.setFixedTime(new Date(`${year}-10-31T17:00:00Z`))
    await calendar(page)
    await expect(page.locator(selectedDate)).toHaveAttribute('data-value', `${year}-11-01`)
    await expect(page.locator('[data-slot="calendar-cell-trigger"][data-today]')).toHaveAttribute(
      'data-value',
      `${year}-11-01`,
    )
    await page.getByRole('button', { name: 'Bulan berikutnya', exact: true }).click()
    await page.getByRole('button', { name: 'Hari ini', exact: true }).click()
    await expect(page.locator(selectedDate)).toHaveAttribute('data-value', `${year}-11-01`)
    await expect(
      page.getByRole('heading', { name: `1 November ${year}`, exact: true }),
    ).toBeVisible()
  })

  test('follows the new school day and year without reloading or overwriting an open form', async ({
    page,
  }) => {
    await page.clock.install({ time: new Date(`${year}-12-31T16:59:50Z`) })
    await calendar(page)
    await expect(page.locator(selectedDate)).toHaveAttribute('data-value', `${year}-12-31`)
    await page.getByRole('button', { name: 'Tambah libur sekolah', exact: true }).click()
    await page.getByLabel('Keterangan', { exact: true }).fill('Rencana libur semester')
    await page.clock.runFor(61_000)
    await expect(page.locator(selectedDate)).toHaveAttribute('data-value', `${year + 1}-01-01`)
    await expect(page.getByLabel('Tanggal mulai', { exact: true })).toHaveValue(`${year}-12-31`)
    await expect(page.getByLabel('Keterangan', { exact: true })).toHaveValue(
      'Rencana libur semester',
    )
    await page.keyboard.press('Escape')
    await expect(
      page.getByRole('heading', { name: `1 Januari ${year + 1}`, exact: true }),
    ).toBeVisible()
  })

  test('catches a changed date when a suspended browser window regains focus', async ({ page }) => {
    await page.clock.setFixedTime(new Date(`${year}-12-31T16:59:50Z`))
    await calendar(page)
    await expect(page.locator(selectedDate)).toHaveAttribute('data-value', `${year}-12-31`)
    await page.clock.setFixedTime(new Date(`${year}-12-31T17:00:10Z`))
    await page.evaluate(() => window.dispatchEvent(new Event('focus')))
    await expect(page.locator(selectedDate)).toHaveAttribute('data-value', `${year + 1}-01-01`)
  })
})
