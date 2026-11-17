import { chromium } from 'playwright-core'
import fs from 'node:fs/promises'
import path from 'node:path'
import { checkNavigationLayout } from './navigation-layout.mjs'

const root = path.resolve(import.meta.dirname, '../..')
const out = path.join(root, 'docs/design')
const browser = await chromium.launch({
  executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  headless: true,
  chromiumSandbox: true,
  args: ['--no-proxy-server'],
})
const errors = [],
  external = [],
  mutations = []
const account = {
  id: 'local_fixture',
  name: '胖乖用户',
  phone: '132****9952',
  updated_at: '2026-11-13T04:00:00Z',
}
const devices = [
  {
    goodsId: 'fixture_water',
    goodsName: '301 饮水机',
    categoryCode: '04',
    workStatus: 10,
    orgName: '嘉应学院·江北校区',
  },
  {
    goodsId: 'fixture_shower',
    goodsName: '9304',
    categoryCode: '08',
    workStatus: 20,
    orgName: '嘉应学院',
  },
]
const hot = {
  skuId: '9007199254740993',
  name: '热水',
  price: '1.00',
  unit: '1.1',
  amount: 1,
  scanAvailable: true,
  billing_unit: '元/L',
  billing_levels: [{ price: '0.28' }],
}
const cold = { ...hot, skuId: 'fixture_cold', name: '冷水', billing_levels: [{ price: '0.10' }] }
let feeChanged = true
let waterPhase = 'idle',
  showerPhase = 'running',
  stateReads = 0
let legacyUnknown = false,
  failUsageRead = false
let waterDeviceBusy = false,
  failWaterOptions = 0
const waterStarted = new Date(Date.now() - 20000).toISOString(),
  showerStarted = new Date(Date.now() - 206000).toISOString()
try {
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
  })
  // All API and static requests are fulfilled from fixtures. No real service is contacted.
  await context.route('**/*', async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      pathname = url.pathname
    if (url.origin !== 'http://127.0.0.1:18080') {
      external.push(url.origin)
      await route.abort()
      return
    }
    if (!pathname.startsWith('/api/')) {
      const directory = path.join(root, 'dist/web'),
        file = path.resolve(
          directory,
          pathname === '/' ? 'index.html' : decodeURIComponent(pathname.slice(1)),
        )
      if (!file.startsWith(directory + path.sep)) {
        await route.abort()
        return
      }
      await route.fulfill({
        status: 200,
        contentType:
          { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css' }[
            path.extname(file)
          ] ?? 'application/octet-stream',
        body: await fs.readFile(file),
      })
      return
    }
    if (req.method() !== 'GET') mutations.push({ path: pathname, body: req.postDataJSON() })
    let body
    if (pathname === '/api/status')
      body = { app: '水到渠成', version: 'fixture', protocol_ready: true, methods: ['sms'] }
    else if (pathname === '/api/accounts') body = { accounts: [account] }
    else if (pathname.endsWith('/tickets'))
      body = {
        wallets: [
          { shopId: 'fixture_a', shopName: '嘉应学院', totalAmount: '4349' },
          { shopId: 'fixture_b', shopName: '嘉应学院·江北校区', totalAmount: '2970' },
        ],
        amount_unit: 'fen',
        currency: 'CNY',
        mode: 'readonly',
      }
    else if (pathname.endsWith('/devices')) {
      devices[0].workStatus = waterDeviceBusy ? 20 : 10
      if (showerPhase === 'finished') devices[1].workStatus = 10
      body = { devices, mode: 'readonly' }
    } else if (pathname.endsWith('/usage/state')) {
      stateReads++
      if (failUsageRead) {
        await route.fulfill({
          status: 502,
          contentType: 'application/json',
          body: JSON.stringify({ message: 'fixture state unavailable' }),
        })
        return
      }
      const water = pathname.includes('fixture_water'),
        state = water ? waterPhase : pathname.includes('fixture_shower') ? showerPhase : 'idle'
      body = {
        session_id: water ? 'fixture_confirmation' : 'fixture_shower_session',
        state,
        can_stop: !water && state === 'running',
        stop_supported: !water,
        started_at: water ? (legacyUnknown ? '0001-01-01T00:00:00Z' : waterStarted) : showerStarted,
        updated_at: new Date().toISOString(),
        service_name: water ? '热水' : '淋浴',
        cost_yuan: null,
        cost_source: 'unavailable',
      }
      if (waterPhase === 'starting' && water) waterPhase = 'running'
      if (showerPhase === 'stopping' && !water) showerPhase = 'finished'
    } else if (pathname.endsWith('/usage/options')) {
      if (pathname.includes('fixture_water') && failWaterOptions > 0) {
        failWaterOptions--
        await route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({ message: '设备结算中，请稍后重试' }),
        })
        return
      }
      body = {
        device: {
          goods_id: 'fixture_water',
          name: '301 饮水机',
          category: '04',
          services: [hot, cold],
        },
        start_supported: true,
        mode: 'readonly',
      }
    } else if (pathname.endsWith('/usage/prepare')) {
      const input = req.postDataJSON()
      if (
        input.sku_id !== hot.skuId ||
        input.coordinate_system !== 'wgs84' ||
        input.latitude !== 22.5 ||
        input.longitude !== 113.5
      )
        throw Error('actual position or opaque SKU changed')
      body = {
        status: 'prepared',
        confirmation_key: 'fixture_confirmation',
        expires_in: 120,
        service: feeChanged ? { ...hot, billing_levels: [{ price: '0.30' }] } : hot,
      }
      if (feeChanged) {
        hot.billing_levels = [{ price: '0.30' }]
        feeChanged = false
      }
    } else if (pathname.endsWith('/usage/start')) {
      const input = req.postDataJSON()
      if (input.confirmation_key !== 'fixture_confirmation' || input.confirm !== true)
        throw Error('confirmation chain missing')
      waterPhase = 'starting'
      body = {
        status: 'accepted',
        session_id: 'fixture_confirmation',
        started_at: waterStarted,
        message: '测试：启动请求已发送',
      }
    } else if (pathname.endsWith('/usage/stop')) {
      if (
        !pathname.includes('fixture_shower') ||
        req.postDataJSON().session_id !== 'fixture_shower_session' ||
        req.postDataJSON().confirm !== true
      )
        throw Error('foreign or unsupported device stopped')
      showerPhase = 'stopping'
      body = { status: 'stopping', message: '测试：终止请求已发送' }
    } else if (pathname.endsWith('/devices/scan')) {
      if (req.postDataJSON().text !== 'SN=fixture-code') throw Error('native QR result changed')
      const device = {
        goodsId: 'fixture_scan',
        goodsName: '扫码设备',
        categoryCode: '04',
        workStatus: 0,
        orgName: '嘉应学院',
      }
      devices.push(device)
      body = { device, started: false }
    } else {
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({ message: 'Unexpected fixture route' }),
      })
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    })
  })
  await context.addInitScript(() => {
    window.qaLocation = 'denied'
    window.qaScan = 'cancelled'
    const reply = (id, data, error) =>
      setTimeout(
        () =>
          window.dispatchEvent(
            new CustomEvent('panguai-native-result', { detail: { id, data, error } }),
          ),
        30,
      )
    window.PanguaiNative = {
      requestLocation(id) {
        reply(
          id,
          {
            latitude: 22.5,
            longitude: 113.5,
            accuracy: 12,
            timestamp: Date.now() - (window.qaLocation === 'stale' ? 120000 : 0),
            coordinate_system: 'wgs84',
          },
          window.qaLocation === 'denied' ? '请在系统权限中允许精确定位后重试' : null,
        )
      },
      scanCode(id) {
        reply(
          id,
          { text: 'SN=fixture-code' },
          window.qaScan === 'cancelled' ? 'scan_cancelled' : null,
        )
      },
    }
  })
  const page = await context.newPage()
  page.on('pageerror', (e) => errors.push(e.message))
  await page.goto('http://127.0.0.1:18080/', { waitUntil: 'networkidle' })
  await page.getByText('0.28 元/L', { exact: true }).waitFor()
  if ((await page.locator('.wallet-row').count()) !== 2) throw Error('wallets combined')
  const walletText = await page.locator('.ticket-card').innerText()
  if (
    !walletText.includes('43.49') ||
    !walletText.includes('29.70') ||
    walletText.includes('4349.00')
  )
    throw Error('fen shown as yuan')
  if ((await page.locator('.app-navbar__title').innerText()) !== '水到渠成')
    throw Error('app name missing')
  const dots = await page
    .locator('.status-pill i')
    .evaluateAll((es) => es.map((e) => getComputedStyle(e).backgroundColor))
  if (dots[0] !== 'rgb(34, 197, 94)' || dots[1] !== 'rgb(239, 68, 68)')
    throw Error('idle/non-idle colors incorrect')
  const shape = await page
    .locator('.device-card')
    .first()
    .evaluate((e) => ({
      color: getComputedStyle(e).backgroundColor,
      curve: getComputedStyle(e, '::before').borderTopLeftRadius,
    }))
  if (shape.color !== 'rgb(240, 242, 245)' || shape.curve === '0px')
    throw Error('card shape/theme lost')
  await page.screenshot({ path: path.join(out, 'home.png') })
  if (await page.getByRole('button', { name: '选择校区' }).count())
    throw Error('campus control retained')
  await page.getByRole('button', { name: '我的', exact: true }).click()
  if (
    (await page.locator('.my-page .profile-header').count()) !== 1 ||
    (await page.getByRole('button', { name: '扫码添加设备' }).count())
  )
    throw Error('My not minimal')
  if (/1[3-9]\d{9}/.test(await page.locator('body').innerText())) throw Error('full phone exposed')
  await page.screenshot({ path: path.join(out, 'my.png') })
  await page.getByRole('button', { name: '首页', exact: true }).click()
  await page.getByRole('button', { name: '冷水', exact: true }).click()
  await page.getByText('0.10 元/L', { exact: true }).waitFor()
  if (mutations.length) throw Error('service selection submitted command')
  await page.getByRole('button', { name: '热水', exact: true }).click()
  const start = page.getByRole('button', { name: '启动 301 饮水机', exact: true })
  await start.click()
  await page.getByText('请在系统权限中允许精确定位后重试', { exact: true }).waitFor()
  if (mutations.length || (await page.locator('.el-drawer:visible').count()))
    throw Error('denied location started or opened drawer')
  await page.evaluate(() => (window.qaLocation = 'stale'))
  await start.click()
  await page
    .getByText('定位精度不足或位置已过期，请到信号较好的位置重试', { exact: true })
    .waitFor()
  if (mutations.length) throw Error('stale location submitted command')
  await page.evaluate(() => (window.qaLocation = 'success'))
  await start.click()
  await page.getByText('服务或费率有变化，请核对后再次点击启动', { exact: true }).waitFor()
  if (mutations.length !== 1 || !mutations[0].path.endsWith('/prepare'))
    throw Error('fee change auto-started')
  await start.click()
  await page.getByText('测试：启动请求已发送', { exact: true }).waitFor()
  await page.waitForLoadState('networkidle')
  if (
    mutations.filter((m) => m.path.endsWith('/start')).length !== 1 ||
    (await page.locator('.el-message-box:visible,.el-drawer:visible').count())
  )
    throw Error('direct start duplicated or modal retained')
  if (await page.getByRole('button', { name: '启动 301 饮水机', exact: true }).count())
    throw Error('accepted start still rendered idle')
  await page.getByRole('button', { name: '查看 301 饮水机', exact: true }).waitFor()
  if (await page.getByRole('button', { name: '终止 301 饮水机', exact: true }).count())
    throw Error('unsupported water end exposed')
  const durationBefore = await page.locator('.usage-duration').first().innerText()
  await page.waitForTimeout(1100)
  if (durationBefore === (await page.locator('.usage-duration').first().innerText()))
    throw Error('duration is static')
  if ((await page.locator('.usage-cost').allTextContents()).some((s) => s !== '待结算'))
    throw Error('unavailable live price fabricated')
  await page.getByText('测试：启动请求已发送', { exact: true }).waitFor({ state: 'hidden' })
  await page.screenshot({ path: path.join(out, 'running.png') })
  await page.locator('.device-list').screenshot({ path: path.join(out, 'usage-cards.png') })
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '查看 301 饮水机', exact: true }).waitFor()
  await page.getByRole('button', { name: '终止 9304', exact: true }).click()
  await page.getByText('测试：终止请求已发送', { exact: true }).waitFor()
  if (await page.getByRole('button', { name: '启动 9304', exact: true }).count())
    throw Error('end acknowledgement treated as physically ended')
  await page.getByRole('button', { name: '启动 9304', exact: true }).waitFor()
  if (mutations.filter((m) => m.path.endsWith('/stop')).length !== 1)
    throw Error('duplicate stop request')
  const beforeScan = mutations.length
  await page.getByRole('button', { name: '扫码添加设备' }).click()
  await page.waitForTimeout(120)
  if (mutations.length !== beforeScan) throw Error('cancelled scan submitted QR')
  await page.evaluate(() => (window.qaScan = 'success'))
  await page.getByRole('button', { name: '扫码添加设备' }).click()
  await page.getByRole('heading', { name: '扫码设备', exact: true }).waitFor()
  if (mutations.length !== beforeScan + 1 || !mutations.at(-1).path.endsWith('/devices/scan'))
    throw Error('scan caused start')
  legacyUnknown = true
  waterPhase = 'unknown'
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await page.getByRole('button', { name: '核对 301 饮水机', exact: true }).waitFor()
  if ((await page.locator('.usage-duration').first().innerText()) !== '--:--')
    throw Error('legacy missing start time rendered as real duration')
  const beforeCheck = mutations.length
  await page.getByRole('button', { name: '核对 301 饮水机', exact: true }).click()
  await page.getByText('平台尚未确认本次使用是否结束，请在官方 App 核对', { exact: true }).waitFor()
  for (let i = 0; i < 3; i++) {
    await page.getByRole('button', { name: '核对 301 饮水机', exact: true }).click()
    await page
      .getByText('平台尚未确认本次使用是否结束，请在官方 App 核对', { exact: true })
      .last()
      .waitFor()
  }
  await page.waitForTimeout(400)
  if ((await page.locator('.el-message--warning:visible').count()) > 1)
    throw Error(
      'repeated verification stacked warning messages: ' +
        JSON.stringify(await page.locator('.el-message--warning:visible').allTextContents()),
    )
  failUsageRead = true
  await page.getByRole('button', { name: '核对 301 饮水机', exact: true }).click()
  await page
    .getByText('核对失败，暂时无法查询平台状态，请稍后重试或在官方 App 查看', { exact: true })
    .waitFor()
  failUsageRead = false
  waterPhase = 'finished'
  await page.getByRole('button', { name: '核对 301 饮水机', exact: true }).click()
  await page.getByText('本地使用记录已解除，设备列表已刷新', { exact: true }).waitFor()
  await page.getByRole('button', { name: '启动 301 饮水机', exact: true }).waitFor()
  if (mutations.length !== beforeCheck) throw Error('manual verification controlled equipment')
  // A physical stop may settle before latestUsed catches up. The end reload
  // skips this busy device's options; a later idle list must fetch them again.
  waterDeviceBusy = true
  waterPhase = 'running'
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '查看 301 饮水机', exact: true }).waitFor()
  waterPhase = 'finished'
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await page.locator('.usage-duration').waitFor({ state: 'hidden' })
  await page.waitForLoadState('networkidle')
  waterDeviceBusy = false
  failWaterOptions = 1
  const retry = page.getByRole('button', { name: '刷新 301 饮水机', exact: true })
  const waterRate = page
    .locator('.device-card')
    .filter({ has: page.getByRole('heading', { name: '301 饮水机', exact: true }) })
    .getByText('0.30 元/L', { exact: true })
  await retry.waitFor({ timeout: 25000 })
  await page.waitForLoadState('networkidle')
  await retry.click()
  await page.getByText('服务信息已更新，请核对服务和费率后再次点击启动', { exact: true }).waitFor()
  await waterRate.waitFor()
  await page.getByRole('button', { name: '启动 301 饮水机', exact: true }).waitFor()
  if (mutations.length !== beforeCheck) throw Error('service recovery controlled equipment')
  // Also cover automatic recovery on the idle transition without a click.
  waterDeviceBusy = true
  waterPhase = 'running'
  await page.reload({ waitUntil: 'networkidle' })
  waterPhase = 'finished'
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await page.locator('.usage-duration').waitFor({ state: 'hidden' })
  await page.waitForLoadState('networkidle')
  waterDeviceBusy = false
  await waterRate.waitFor({ timeout: 25000 })
  await page.getByRole('button', { name: '启动 301 饮水机', exact: true }).waitFor()
  if (mutations.length !== beforeCheck) throw Error('idle recovery controlled equipment')
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await page.waitForTimeout(150)
  const hiddenReads = stateReads
  await page.waitForTimeout(4200)
  if (stateReads !== hiddenReads) throw Error('polling continued in background')
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, value: false })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await page.setViewportSize({ width: 320, height: 740 })
  if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth))
    throw Error('small-screen horizontal overflow')
  const navigationLayouts = await checkNavigationLayout(page, out)
  const board = await context.newPage()
  await board.setViewportSize({ width: 1060, height: 1020 })
  const imgs = await Promise.all(
    ['home', 'my'].map(
      async (n) =>
        'data:image/png;base64,' +
        (await fs.readFile(path.join(out, n + '.png'))).toString('base64'),
    ),
  )
  await board.setContent(
    `<html><meta charset="utf-8"><style>*{box-sizing:border-box}body{margin:0;background:#eff1f4;font-family:'Microsoft YaHei',sans-serif;color:#353b46;padding:32px 90px}h1{font-size:20px;margin:0 0 22px}main{display:flex;gap:60px}article{width:390px}img{display:block;width:390px;border:5px solid white;border-radius:25px;box-shadow:0 10px 35px #353b4610}p{text-align:center;font-size:13px}small{display:block;font-size:11px;color:#8a94a3;margin-top:8px}</style><h1>水到渠成 · 首页与我的<small>小票金额已换算 · 原卡片结构 · 空闲绿点，其他状态红点</small></h1><main>${imgs.map((src, i) => `<article><img src="${src}"><p>${['首页', '我的'][i]}</p></article>`).join('')}</main><small>实际 Vue 页面 · 示例数据 · 所有网络请求已拦截，未操作真实设备</small></html>`,
  )
  await board.screenshot({ path: path.join(out, 'current.png'), fullPage: true })
  const result = {
    appName: true,
    navbar: true,
    navigationLayouts,
    walletFenConversion: true,
    separateWallets: true,
    originalCardShape: true,
    neutralTheme: true,
    idleGreenOtherRed: true,
    accountProfilePresent: true,
    fullPhoneHidden: true,
    campusRemoved: true,
    acceptedStartOverridesStaleIdle: true,
    durationTicks: true,
    noFabricatedLiveMoney: true,
    sessionRestoresAfterReload: true,
    stopSingleRequest: true,
    endWaitsForPlatform: true,
    unsupportedWaterEndHidden: true,
    pollingPausesInBackground: true,
    legacyUnknownDoesNotInventTime: true,
    manualCheckShowsResultAndFailure: true,
    repeatedCheckDoesNotStackWarnings: true,
    confirmedEndRestoresStartOptions: true,
    delayedIdleRestoresServiceOptions: true,
    failedServiceReadCanRetryWithoutStarting: true,
    inlineServiceSelection: true,
    deniedLocationNoRequest: true,
    staleLocationNoRequest: true,
    feeChangeNoStart: true,
    directStartNoModal: true,
    scanCancelNoRequest: true,
    scanAddsWithoutStart: true,
    smallViewportOverflow: false,
    realMutationsSent: false,
    errors,
    externalRequests: external,
  }
  if (errors.length || external.length) throw Error(JSON.stringify(result))
  await fs.writeFile(path.join(out, 'checks.json'), JSON.stringify(result, null, 2))
  console.log(JSON.stringify(result))
  await context.close()
} finally {
  await browser.close()
}
