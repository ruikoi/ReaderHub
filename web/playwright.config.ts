import { defineConfig } from '@playwright/test'
export default defineConfig({
 testDir: './tests', workers: 1, fullyParallel: false, timeout: 30000,
 outputDir: '../.local/browser-results', reporter: 'list',
 use: { baseURL: process.env.READERHUB_TEST_URL || 'http://127.0.0.1:18848', channel: 'chrome', headless: true, screenshot: 'only-on-failure' },
 projects: [
  { name: 'desktop', use: { viewport: { width: 1440, height: 1000 } } },
  { name: 'ipad-portrait', use: { viewport: { width: 768, height: 1024 }, isMobile: true, hasTouch: true } },
  { name: 'android-tablet', use: { viewport: { width: 1280, height: 800 }, isMobile: true, hasTouch: true } }
 ]
})
