import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright-core'

const root = path.resolve(import.meta.dirname, '../..')
const out = path.join(root, 'docs/design/about-implemented')
const metadata = JSON.parse(await fs.readFile(path.join(root, 'package.json'), 'utf8'))
const origin = 'http://127.0.0.1:18081'
const errors = [],
  unexpected = []
const pages = [
  ['license', '开源许可'],
  ['sponsor', '赞助'],
  ['disclaimer', '免责声明'],
  ['contact', '联系我们'],
]
await fs.mkdir(out, { recursive: true })
const browser = await chromium.launch({
  executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  headless: true,
  chromiumSandbox: true,
})
try {
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
  })
  context.setDefaultTimeout(15000)
  context.setDefaultNavigationTimeout(15000)
  await context.route('**/*', async (route) => {
    const request = route.request(),
      url = new URL(request.url())
    if (url.origin !== origin || request.method() !== 'GET') {
      unexpected.push(request.method() + ' ' + url.pathname)
      await route.abort()
      return
    }
    if (url.pathname.startsWith('/api/')) {
      const fixtures = {
        '/api/status': {
          app: metadata.panguai.displayName,
          version: metadata.version,
          protocol_ready: false,
          methods: [],
        },
        '/api/accounts': { accounts: [] },
      }
      const body = fixtures[url.pathname]
      if (!body) unexpected.push(url.pathname)
      await route.fulfill({ json: body ?? {}, status: body ? 200 : 404 })
      return
    }
    const directory = path.join(root, 'dist/web')
    const file = path.resolve(
      directory,
      url.pathname === '/' ? 'index.html' : decodeURIComponent(url.pathname.slice(1)),
    )
    if (!file.startsWith(directory + path.sep)) throw Error('Static path escaped')
    const body = await fs.readFile(file)
    await route.fulfill({
      body,
      contentType:
        {
          '.html': 'text/html',
          '.js': 'text/javascript',
          '.css': 'text/css',
          '.jpg': 'image/jpeg',
        }[path.extname(file)] ?? 'application/octet-stream',
    })
  })
  await context.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: async (value) => {
          window.qaClipboard = value
        },
      },
    })
  })
  const page = await context.newPage()
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto(origin)
  assert.equal(await page.title(), '水到渠成')
  assert.equal(await page.locator('.app-navbar__title').innerText(), '水到渠成')
  await page.getByRole('button', { name: '我的', exact: true }).click()
  await page.getByRole('button', { name: '账号资料' }).waitFor()
  assert.equal(await page.locator('.my-menu-item').count(), 4)
  assert.match(await page.locator('.my-version').innerText(), /水到渠成/)
  await page.screenshot({ path: path.join(out, 'my.png'), fullPage: true })

  for (const [id, title] of pages) {
    console.log(`[about-ui] ${id}`)
    await page.locator('.my-menu-item').filter({ hasText: title }).click()
    await page.waitForURL(`**/#/my/${id}`)
    assert.equal(await page.locator('.app-navbar__title').innerText(), title)
    assert.equal(await page.locator('.bottom-nav').count(), 0)
    for (const width of [320, 390, 480, 900]) {
      await page.setViewportSize({ width, height: 844 })
      assert.equal(
        await page.evaluate(() => document.documentElement.scrollWidth > innerWidth),
        false,
        `${title}: overflow at ${width}`,
      )
      const image = page.locator('.about-page img')
      if (await image.count())
        assert.equal(await image.evaluate((el) => el.complete && el.naturalWidth > 0), true)
    }
    await page.setViewportSize({ width: 390, height: 844 })
    if (id === 'license') {
      assert.equal(
        await page.getByRole('link', { name: '访问开源仓库' }).getAttribute('href'),
        'https://github.com/842720301wh-ship-it/pangguai',
      )
    }
    if (id === 'sponsor' || id === 'contact') {
      const downloadPromise = page.waitForEvent('download')
      await page
        .getByRole('button', { name: id === 'sponsor' ? '保存赞赏码' : '保存二维码', exact: true })
        .click()
      const download = await downloadPromise
      assert.equal(
        download.suggestedFilename(),
        `水到渠成-${id === 'sponsor' ? '赞赏码' : 'QQ群'}.jpg`,
      )
      const saved = await fs.readFile(await download.path())
      const original = await fs.readFile(path.join(root, `frontend/public/about/${id}.jpg`))
      assert.deepEqual(saved, original, `${id}: original image download changed`)
      await page.locator('.el-message').waitFor({ state: 'hidden' })
    }
    if (id === 'contact') {
      await page.getByRole('button', { name: '复制群号' }).click()
      assert.equal(await page.evaluate(() => window.qaClipboard), '345353411')
      await page.locator('.el-message').waitFor({ state: 'hidden' })
    }
    await page.screenshot({ path: path.join(out, `${id}.png`), fullPage: true })
    await page.getByRole('button', { name: '返回我的' }).click()
    await page.waitForURL('**/#/my')
    assert.equal(await page.locator('.bottom-nav').count(), 1)
  }
  // The browser/Android history must return to My rather than reopen an earlier detail.
  console.log('[about-ui] browser history')
  await page.locator('.my-menu-item').filter({ hasText: '免责声明' }).click()
  await page.getByRole('button', { name: '「联系我们」' }).click()
  await page.waitForURL('**/#/my/contact')
  await page.goBack()
  console.log(`[about-ui] back: ${page.url()}`)
  await page.waitForURL('**/#/my')
  await page.goForward()
  console.log(`[about-ui] forward: ${page.url()}`)
  await page.waitForURL('**/#/my/contact')
  await page.reload()
  assert.equal(await page.locator('.app-navbar__title').innerText(), '联系我们')
  await page.getByRole('button', { name: '返回我的' }).click()
  await page.waitForURL('**/#/my')
  // Direct navigation has no in-app parent in history; top back still goes to My.
  const direct = await context.newPage()
  console.log('[about-ui] direct entry')
  await direct.goto(origin + '/#/my/license')
  await direct.getByRole('button', { name: '返回我的' }).click()
  await direct.waitForURL('**/#/my')
  await direct.close()

  // Exercise native bridge arguments and cancel/failure feedback separately from browser downloads.
  console.log('[about-ui] native bridge')
  const nativePage = await context.newPage()
  await nativePage.addInitScript(() => {
    window.qaNativeCalls = []
    const reply = (id, error) =>
      setTimeout(
        () =>
          window.dispatchEvent(
            new CustomEvent('panguai-native-result', { detail: { id, data: {}, error } }),
          ),
        20,
      )
    window.PanguaiNative = {
      saveImage(id, key) {
        window.qaNativeCalls.push(key)
        reply(id, window.qaSaveError ?? null)
      },
      copyGroupNumber(id) {
        window.qaNativeCalls.push('copy-group')
        reply(id, null)
      },
    }
  })
  await nativePage.goto(origin + '/#/my/contact')
  console.log(
    '[about-ui] native initialized',
    await nativePage.evaluate(() => typeof window.PanguaiNative?.saveImage),
  )
  await nativePage.getByRole('button', { name: '复制群号' }).click()
  console.log('[about-ui] native copy invoked')
  await nativePage.getByRole('button', { name: '保存二维码' }).click()
  console.log('[about-ui] native save invoked')
  await nativePage.getByText('图片已保存', { exact: true }).waitFor()
  console.log('[about-ui] native save confirmed')
  assert.deepEqual(await nativePage.evaluate(() => window.qaNativeCalls), ['copy-group', 'contact'])
  await nativePage.locator('.el-message').last().waitFor({ state: 'hidden' })
  console.log('[about-ui] native messages cleared')
  await nativePage.evaluate(() => {
    window.qaSaveError = 'save_cancelled'
  })
  await nativePage.getByRole('button', { name: '保存二维码' }).click()
  await nativePage.getByRole('button', { name: '保存二维码' }).waitFor()
  await nativePage.waitForFunction(
    () => !document.querySelector('.about-button-row button').disabled,
  )
  assert.equal(
    await nativePage.locator('.el-message').count(),
    0,
    'Save cancellation should be silent',
  )
  await nativePage.evaluate(() => {
    window.qaSaveError = '图片保存失败，请重新选择保存位置'
  })
  await nativePage.getByRole('button', { name: '保存二维码' }).click()
  await nativePage.getByText('图片保存失败，请重新选择保存位置', { exact: true }).waitFor()

  const board = await context.newPage()
  await board.setViewportSize({ width: 2168, height: 1030 })
  const images = await Promise.all(
    ['my', ...pages.map(([id]) => id)].map(
      async (id) =>
        'data:image/png;base64,' +
        (await fs.readFile(path.join(out, `${id}.png`))).toString('base64'),
    ),
  )
  await board.setContent(
    `<html lang="zh-CN"><meta charset="utf-8"><style>*{box-sizing:border-box}body{margin:0;padding:30px 40px;background:#e9ecf0;font-family:'Microsoft YaHei',sans-serif;color:#353b46}h1{font-size:24px;margin:0 0 12px}p{font-size:13px;color:#8390a0;margin:0 0 22px}main{display:flex;gap:26px}article{width:390px}h2{font-size:14px;margin:0 0 12px}img{width:390px;border:1px solid #dae0e8;border-radius:24px;display:block}</style><h1>水到渠成 · 我的与四个详情页</h1><p>实际 Vue 页面 · 未登录也可访问 · 示例页面检查</p><main>${images.map((src, i) => `<article><h2>${['我的', ...pages.map(([, title]) => title)][i]}</h2><img src="${src}"></article>`).join('')}</main></html>`,
  )
  await board.screenshot({ path: path.join(out, 'overview.png'), fullPage: true })
  assert.deepEqual(errors, [])
  assert.deepEqual(unexpected, [])
  const result = {
    entries: 4,
    noLoginRequired: true,
    navigationAndHistory: true,
    reloadAndDirectEntry: true,
    originalImageDownloads: true,
    clipboard: true,
    nativeBridgeAndSaveErrors: true,
    widths: [320, 390, 480, 900],
    externalOrMutationRequests: unexpected,
    errors,
  }
  await fs.writeFile(path.join(out, 'checks.json'), JSON.stringify(result, null, 2))
  console.log(JSON.stringify(result))
} catch (error) {
  console.error(error)
  throw error
} finally {
  await browser.close()
}
