// End-to-end demo against the real ShopHub backend (port-forward :8090):
// register -> dashboard -> create a shop -> screenshot. Throwaway user each run.
import { chromium } from 'playwright';

const base = 'http://localhost:5174';
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

await page.goto(`${base}/login`, { waitUntil: 'networkidle' });
await page.getByText('Sign up').click(); // login -> register mode
const email = `demo+${Date.now()}@example.com`;
await page.locator('input[type="email"]').fill(email);
await page.locator('input[type="password"]').fill('hunter2pass');
await page.getByRole('button', { name: /create account/i }).click();

await page.waitForURL('**/dashboard', { timeout: 20000 });
await page.waitForTimeout(800);

await page.getByRole('button', { name: /new shop/i }).first().click();
await page.locator('input[placeholder="my-store"]').fill('demo-store');
await page.locator('input[placeholder="My Store"]').fill('Demo Store');
await page.locator('input[placeholder^="0x"]').fill('0x3a6B1512a8ccF0315c0A392E98975Ac659D24e06');
await page.getByRole('button', { name: /create shop/i }).click();
await page.waitForTimeout(2500);

await page.screenshot({ path: 'render-dashboard.png', fullPage: true });
await browser.close();
console.log('demo done as', email);
