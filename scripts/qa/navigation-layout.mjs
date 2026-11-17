import assert from 'node:assert/strict'
import path from 'node:path'

// Run against the existing fully mocked UI session; never contact a real device.
export async function checkNavigationLayout(page, out) {
  const cases = [
    { name: 'compact', width: 320, height: 568, top: 0, bottom: 0 },
    { name: 'android-buttons', width: 360, height: 640, top: 24, bottom: 24 },
    { name: 'notch', width: 390, height: 844, top: 47, bottom: 34 },
    { name: 'android-gestures', width: 430, height: 932, top: 32, bottom: 0 },
    { name: 'keyboard-resize', width: 390, height: 360, top: 24, bottom: 0 },
    { name: 'landscape', width: 844, height: 390, top: 0, bottom: 21, left: 44, right: 44 },
    { name: 'desktop', width: 1280, height: 900, top: 0, bottom: 0 },
  ]
  async function geometry() {
    return page.evaluate(() => {
      const rect = (selector) => {
        const { top, bottom, left, right, height } = document
          .querySelector(selector)
          .getBoundingClientRect()
        return { top, bottom, left, right, height }
      }
      return {
        frame: {
          ...rect('.app-frame'),
          borderBottom: parseFloat(
            getComputedStyle(document.querySelector('.app-frame')).borderBottomWidth,
          ),
        },
        header: rect('.app-navbar'),
        toolbar: rect('.app-navbar__content'),
        main: rect('.app-main'),
        nav: rect('.bottom-nav'),
        button: rect('.bottom-nav button'),
        title: rect('.app-navbar__title'),
        horizontalOverflow: document.documentElement.scrollWidth > innerWidth,
        contentOverflow:
          document.querySelector('.app-main').scrollWidth >
          document.querySelector('.app-main').clientWidth,
        documentScroll: document.documentElement.scrollHeight > innerHeight,
      }
    })
  }
  for (const config of cases) {
    await page.setViewportSize({ width: config.width, height: config.height })
    await page.evaluate(({ top, bottom, left = 0, right = 0 }) => {
      for (const [edge, value] of Object.entries({ top, bottom, left, right }))
        document.documentElement.style.setProperty(`--safe-area-${edge}`, `${value}px`)
    }, config)
    const layout = await geometry()
    const label = config.name
    assert.equal(layout.horizontalOverflow, false, `${label}: horizontal overflow`)
    assert.equal(layout.contentOverflow, false, `${label}: horizontal content overflow`)
    assert.equal(layout.documentScroll, false, `${label}: document scrolls outside app`)
    assert.equal(layout.toolbar.top - layout.header.top, config.top, `${label}: top safe area`)
    assert.equal(layout.main.top, layout.header.bottom, `${label}: gap below header`)
    assert.equal(layout.main.bottom, layout.nav.top, `${label}: navigation overlaps content`)
    assert.equal(
      layout.nav.bottom,
      layout.frame.bottom - layout.frame.borderBottom,
      `${label}: gap below navigation`,
    )
    assert.equal(
      layout.nav.bottom - layout.button.bottom,
      8 + config.bottom,
      `${label}: bottom safe area`,
    )
    assert.ok(layout.title.left >= layout.toolbar.left, `${label}: title outside toolbar`)
    assert.ok(layout.title.right <= layout.toolbar.right, `${label}: title outside toolbar`)
    assert.ok(layout.button.height >= 48, `${label}: insufficient navigation touch target`)

    await page.locator('.app-main').evaluate((element) => {
      element.scrollTop = element.scrollHeight
    })
    const last = await page.locator('.device-card').last().boundingBox()
    assert.ok(last.y + last.height <= layout.nav.top, `${label}: last card obscured`)
    const afterScroll = await geometry()
    assert.deepEqual(afterScroll.header, layout.header, `${label}: header moved while scrolling`)
    assert.deepEqual(afterScroll.nav, layout.nav, `${label}: navigation moved while scrolling`)

    await page.getByRole('button', { name: '我的', exact: true }).click()
    assert.equal(await page.locator('.app-main').evaluate((element) => element.scrollTop), 0)
    await page.getByRole('button', { name: /开源许可/ }).click()
    assert.equal(await page.locator('.bottom-nav').count(), 0)
    const detail = await page.locator('.app-main').boundingBox()
    assert.equal(
      detail.y + detail.height,
      layout.frame.bottom - layout.frame.borderBottom - config.bottom,
      `${label}: hidden navigation reserves space`,
    )
    await page.getByRole('button', { name: '返回我的', exact: true }).click()
    await page.getByRole('button', { name: '首页', exact: true }).click()
    assert.equal(await page.locator('.app-main').evaluate((element) => element.scrollTop), 0)
  }

  await page.setViewportSize({ width: 390, height: 844 })
  await page.evaluate(() => {
    document.documentElement.style.cssText = ''
  })
  const regular = await geometry()
  const largeText = await page.addStyleTag({
    content:
      '.app-navbar__title { font-size: 36px; line-height: 1.5 } .bottom-nav button { font-size: 24px; line-height: 1.5 }',
  })
  const enlarged = await geometry()
  assert.ok(enlarged.header.height > regular.header.height, 'header did not grow with text')
  assert.ok(enlarged.nav.height > regular.nav.height, 'navigation did not grow with text')
  assert.equal(enlarged.main.bottom, enlarged.nav.top, 'large text overlaps content')
  await largeText.evaluate((element) => element.remove())
  await page.screenshot({ path: path.join(out, 'navigation-mobile.png') })
  return cases.map(({ name }) => name).concat('large-text')
}
