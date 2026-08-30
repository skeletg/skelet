import { test as setup } from '@playwright/test'
import { AUTH_FILE, login } from './helpers'

// Runs once, ahead of every spec - see helpers.login for why the suite shares one session
setup('sign in', async ({ page }) => {
  await login(page)
  await page.context().storageState({ path: AUTH_FILE })
})
