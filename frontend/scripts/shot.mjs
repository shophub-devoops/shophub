// Headless screenshot for the self-review loop (see repo-root frontend_guide.md).
//   node scripts/shot.mjs <url> <out.png> [width]
import { chromium } from 'playwright';

const [, , url = 'http://localhost:5174/', out = 'render.png', width = '1440'] = process.argv;

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: +width, height: 900 } });
await page.goto(url, { waitUntil: 'networkidle' });
await page.waitForTimeout(1000); // hero entrance

// Scroll through so framer-motion whileInView sections actually render
// (a full-page screenshot otherwise captures them at opacity:0).
const height = await page.evaluate(() => document.body.scrollHeight);
for (let y = 0; y < height; y += 500) {
  await page.evaluate((v) => window.scrollTo(0, v), y);
  await page.waitForTimeout(120);
}
await page.evaluate(() => window.scrollTo(0, 0));
await page.waitForTimeout(500);

await page.screenshot({ path: out, fullPage: true });
await browser.close();
console.log(`wrote ${out} from ${url}`);
