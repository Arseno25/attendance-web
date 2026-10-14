import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript } = requireWeb('@vue/compiler-sfc')
const { h } = requireWeb('vue')
const { renderToString } = requireWeb('@vue/server-renderer')
const directory = resolve('.tools/chart-regression')
mkdirSync(directory, { recursive: true })
writeFileSync(directory + '/package.json', JSON.stringify({ type: 'module' }))
try { symlinkSync(resolve('apps/web/node_modules'), directory + '/node_modules', 'dir') }
catch (error) { if (error.code !== 'EEXIST') throw error }
const { descriptor } = parse(readFileSync('apps/web/app/components/ui/chart/ChartTooltipContent.vue', 'utf8'))
const script = compileScript(descriptor, { id: 'tooltip-regression', inlineTemplate: true, templateOptions: { ssr: true } })
const filename = directory + '/tooltip.ts'
writeFileSync(filename, script.content.replaceAll('@/lib/utils', pathToFileURL(resolve('apps/web/app/lib/utils.ts')).href))
const { default: Tooltip } = await import(pathToFileURL(filename).href)

test('native chart tooltip displays zero counts alongside nonzero values', async () => {
  const html = await renderToString(h(Tooltip, {
    config: { present: { label: 'Tepat waktu' }, late: { label: 'Terlambat' } },
    payload: { present: 1, late: 0 },
  }))
  assert.match(html, /Tepat waktu/)
  assert.match(html, /Terlambat/)
  assert.match(html, />1<\/span>/)
  assert.match(html, />0<\/span>/)
})
process.on('exit', () => rmSync(directory, { recursive: true, force: true }))
