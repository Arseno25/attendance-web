import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('Email', { exact: true }).fill('browser-admin@example.test')
  await page.getByLabel('Password', { exact: true }).fill('BrowserTest1!')
  await page.getByRole('button', { name: 'Masuk', exact: true }).click()
  await expect(page).toHaveURL(/\/admin$/)
}

test('security headers deny framing while nonce-authorized startup and API access work', async ({
  page,
}) => {
  const violations: string[] = []
  await page.addInitScript(() =>
    document.addEventListener('securitypolicyviolation', (e) =>
      console.error(
        `CSP violation ${e.violatedDirective} ${e.blockedURI} ${e.sourceFile}:${e.lineNumber}`,
      ),
    ),
  )
  page.on('console', (m) => {
    if (m.text().includes('CSP violation')) violations.push(m.text())
  })
  const response = await page.goto('/login')
  expect(response!.headers()['x-frame-options']).toBe('DENY')
  expect(response!.headers()['cache-control']).toBe('no-store')
  expect(response!.headers()['content-security-policy']).toContain("frame-ancestors 'none'")
  expect(response!.headers()['content-security-policy']).toMatch(
    /script-src 'self' 'nonce-[A-Za-z0-9+/=]+'/,
  )
  expect(response!.headers()['referrer-policy']).toBe('same-origin')
  await login(page)
  await expect(page.getByRole('heading', { name: 'Ringkasan kehadiran' })).toBeVisible()
  await page.goto('/admin/settings')
  await page.getByRole('tab', { name: 'Tampilan', exact: true }).click()
  await page.getByRole('combobox', { name: 'Tema', exact: true }).click()
  await page.getByRole('option', { name: 'Gelap', exact: true }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)
  expect(violations).toEqual([])
})

test('student and device header actions fit narrow viewports and remain operable', async ({
  page,
}) => {
  await login(page)
  await page.goto('/admin/students')
  const details = page.getByRole('link', { name: /^Detail:/ }).first()
  await expect(details).toBeVisible()
  const path = await details.getAttribute('href')
  for (const width of [320, 390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    for (const route of [path!, '/admin/devices']) {
      await page.goto(route)
      await expect(page.locator('h1')).toBeVisible()
      await expect
        .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
        .toBeLessThanOrEqual(width)
      const actions = page
        .locator('main header')
        .getByRole('button')
        .or(page.locator('main header').getByRole('link'))
      for (const action of await actions.all()) {
        const box = await action.boundingBox()
        if (box) {
          expect(box.x).toBeGreaterThanOrEqual(0)
          expect(box.x + box.width).toBeLessThanOrEqual(width)
        }
      }
    }
  }
  await page.setViewportSize({ width: 320, height: 740 })
  await page.goto(path!)
  await page.getByRole('button', { name: 'Penempatan kelas', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Penempatan kelas' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Penempatan kelas', exact: true })).toBeFocused()
})

test('role rows use named shared actions and preserve delete confirmation', async ({ page }) => {
  await login(page)
  await page.goto('/admin/roles')
  await page.getByRole('button', { name: 'Edit peran: Admin', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Edit peran', exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Hapus peran: Admin', exact: true }).click()
  await expect(page.getByRole('alertdialog')).toBeVisible()
  await page.getByRole('button', { name: 'Batal', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Hapus peran: Admin', exact: true })).toBeFocused()
})

test('public service status hydrates without mismatches', async ({ page }) => {
  const errors: string[] = []
  page.on('console', (message) => {
    if (/hydration|mismatch/i.test(message.text())) errors.push(message.text())
  })
  await page.goto('/')
  await expect(page.locator('main').getByRole('status')).toHaveText('Layanan terhubung')
  expect(errors).toEqual([])
})

test('chart title, description and report action stack on narrow screens', async ({ page }) => {
  await login(page)
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/admin')
    const card = page
      .locator('[data-slot="card"]')
      .filter({ has: page.getByRole('heading', { name: 'Kehadiran tujuh hari', exact: true }) })
    const title = await card.locator('[data-slot="card-title"]').boundingBox()
    const description = await card.locator('[data-slot="card-description"]').boundingBox()
    const action = await card
      .getByRole('link', { name: 'Lihat laporan', exact: true })
      .boundingBox()
    expect(description!.y).toBeGreaterThanOrEqual(title!.y + title!.height)
    expect(action!.y).toBeGreaterThanOrEqual(description!.y + description!.height)
  }
})
