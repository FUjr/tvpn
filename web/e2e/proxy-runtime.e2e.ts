import { expect, test } from '@playwright/test'

const bootstrapURL = process.env.TVPN_E2E_BOOTSTRAP_URL

test('proxy runtime carries fetch, cookies, text and binary WebSocket frames', async ({ page }) => {
  test.skip(!bootstrapURL, 'TVPN_E2E_BOOTSTRAP_URL is not configured')
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.goto(bootstrapURL!)
  await expect(page.locator('#fetch')).toHaveText('fetch-ok')
  await expect(page.locator('#cookie')).toHaveText('cookie-ok')
  await expect(page.locator('#cookie-js')).toHaveText('document-cookie-ok')
  await expect(page.locator('#xhr')).toHaveText('xhr-ok')
  await expect(page.locator('#sse')).toHaveText('sse-ok')
  await expect(page.locator('#socket')).toHaveText('echo-hello:tvpn-echo')
  await expect(page.locator('#binary')).toHaveText('101,99,104,111,45,1,2,3')
  await page.locator('#redirect').click()
  await expect(page.locator('body')).toContainText('redirect-ok')
  expect(page.url()).toMatch(/\.proxy\.localhost:18888\/final$/)
  expect(errors).toEqual([])
})
