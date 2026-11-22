import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, stat } from 'node:fs/promises';
import { extname, resolve, sep } from 'node:path';
import { gzipSync } from 'node:zlib';
import { chromium } from 'playwright';

const root = resolve(process.argv[2] || 'public');
const types = { '.html': 'text/html; charset=utf-8', '.css': 'text/css', '.js': 'text/javascript', '.svg': 'image/svg+xml', '.webp': 'image/webp', '.png': 'image/png', '.jpg': 'image/jpeg' };
const cache = new Map();
const prefetched = new Set();
const server = createServer(async (request, response) => {
  try {
    const { pathname } = new URL(request.url, 'http://localhost');
    if (request.headers['sec-purpose']?.includes('prefetch')) prefetched.add(pathname);
    let path = resolve(root, `.${decodeURIComponent(pathname)}`);
    if (path !== root && !path.startsWith(root + sep)) throw new Error('invalid path');
    if ((await stat(path)).isDirectory()) {
      if (!pathname.endsWith('/')) return response.writeHead(301, { Location: pathname + '/' }).end();
      path = resolve(path, 'index.html');
    }
    const extension = extname(path);
    const content = cache.get(path) || await readFile(path);
    cache.set(path, content);
    const compress = ['.html', '.css', '.js', '.svg'].includes(extension) && request.headers['accept-encoding']?.includes('gzip');
    const body = compress ? gzipSync(content) : content;
    response.writeHead(200, {
      'Content-Type': types[extension] || 'application/octet-stream',
      'Content-Length': body.length,
      'Cache-Control': extension === '.html' ? 'max-age=0' : 'max-age=3600',
      'Vary': 'Accept-Encoding',
      ...(compress ? { 'Content-Encoding': 'gzip' } : {}),
    }).end(body);
  } catch {
    response.writeHead(404).end();
  }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const baseline = process.env.AUDIT_BASELINE === '1';
const sample = [];
let browser;
try {
  browser = await chromium.launch({
    executablePath: process.env.AUDIT_CHROMIUM || undefined,
    ignoreDefaultArgs: ['--disable-back-forward-cache'],
  });
  for (const mobile of [true, false]) {
    for (let run = 0; run < 3; run++) {
      const context = await browser.newContext({
        viewport: mobile ? { width: 390, height: 844 } : { width: 1440, height: 900 },
        isMobile: mobile, hasTouch: mobile, deviceScaleFactor: 1,
      });
      const failures = [];
      const redirects = [];
      context.on('response', response => {
        if (response.status() >= 400 && response.url().startsWith(origin)) failures.push(response.url());
        if (response.request().resourceType() === 'document' && response.status() >= 300 && response.status() < 400) redirects.push(response.url());
      });
      await context.addInitScript(() => {
        window.audit = { lcp: 0, cls: 0, restored: false };
        addEventListener('pageshow', event => { window.audit.restored = event.persisted; });
        new PerformanceObserver(list => {
          for (const entry of list.getEntries()) window.audit.lcp = entry.startTime;
        }).observe({ type: 'largest-contentful-paint', buffered: true });
        let first = 0, last = 0, value = 0;
        new PerformanceObserver(list => {
          for (const entry of list.getEntries()) {
            if (entry.hadRecentInput) continue;
            if (entry.startTime - last >= 1000 || entry.startTime - first >= 5000) {
              first = entry.startTime;
              value = 0;
            }
            value += entry.value;
            last = entry.startTime;
            window.audit.cls = Math.max(window.audit.cls, value);
          }
        }).observe({ type: 'layout-shift', buffered: true });
      });
      const page = await context.newPage();
      const cdp = await context.newCDPSession(page);
      if (mobile) {
        await cdp.send('Network.emulateNetworkConditions', {
          offline: false, latency: 150, downloadThroughput: 1_600_000 / 8, uploadThroughput: 750_000 / 8,
        });
        await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 });
      }
      await page.goto(origin);
      await page.waitForFunction(() => window.audit.lcp > 0);
      const home = await page.evaluate(() => ({
        ...window.audit,
        bytes: [...performance.getEntriesByType('resource'), ...performance.getEntriesByType('navigation')]
          .reduce((sum, entry) => sum + entry.encodedBodySize, 0),
      }));
      const startNavigation = async (locator, trigger) => {
        // Capture click time in the outgoing document, not when Playwright begins waiting.
        if (locator) await locator.evaluate(link => link.addEventListener('click', () => {
          sessionStorage.setItem('audit-click', String(performance.timeOrigin + performance.now()));
        }, { once: true }));
        await trigger();
        return page.evaluate(async () => {
          await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
          return Math.round(performance.timeOrigin + performance.now() - Number(sessionStorage.getItem('audit-click')));
        });
      };
      const entry = page.locator('.post-link').first();
      await entry.scrollIntoViewIfNeeded();
      const scroll = await page.evaluate(() => scrollY);
      const article = await startNavigation(entry, async () => {
        await entry.click();
        await page.locator('.postTitle').waitFor();
      });
      const publicArticle = page.url();
      const nextLink = page.locator('.postNav a').first();
      const neighbor = await startNavigation(nextLink, async () => {
        await nextLink.click();
        await page.locator('.postTitle').waitFor();
      });
      assert.notEqual(page.url(), publicArticle);
      await page.goBack({ waitUntil: 'commit' });
      await page.goBack({ waitUntil: 'commit' });
      if (!baseline) await page.waitForFunction(() => window.audit?.restored, null, { timeout: 3000 });
      const back = await page.evaluate(() => ({ restored: window.audit.restored, scroll: scrollY }));
      sample.push({ mobile, run: run + 1, lcpMs: Math.round(home.lcp), cls: home.cls, homeBytes: home.bytes, articleMs: article, neighborMs: neighbor, bfcache: back.restored });
      if (!baseline) {
        assert.equal(page.url(), `${origin}/`);
        assert.ok(home.lcp <= 2500 && home.cls <= 0.1, JSON.stringify(home));
        assert.ok(home.bytes <= 20_000, `Homepage transfer: ${home.bytes}`);
        assert.ok(article <= 1200 && neighbor <= 1200, `Navigation: ${article}/${neighbor} ms`);
        assert.ok(back.restored, 'Homepage should be restored from bfcache');
        assert.ok(Math.abs(back.scroll - scroll) < 4, `scroll position: ${back.scroll}/${scroll}`);
        assert.deepEqual(redirects, []);
      }
      if (!baseline && run === 0) {
        const sameOrigin = await page.locator('header a, .pagination a, footer a[href^="/"]').evaluateAll(links => links.every(a => a.origin === location.origin && !['/index.html', '/about'].includes(a.pathname)));
        assert.ok(sameOrigin);
        const nextPage = page.locator('.pagination a[href="/blog/page/2/"]');
        await nextPage.click();
        assert.equal(page.url(), `${origin}/blog/page/2/`);
        assert.equal(await page.locator('link[rel="canonical"]').getAttribute('href'), 'https://blog.xiaoshae.cn/blog/page/2/');
        if (!mobile && await page.evaluate(() => HTMLScriptElement.supports('speculationrules'))) {
          prefetched.clear();
          await page.locator('a[data-prefetch][href="/blog/Volatility-3/"]').hover();
          for (let retry = 0; retry < 20 && !prefetched.size; retry++) {
            await new Promise(resolve => setTimeout(resolve, 100));
          }
          assert.deepEqual([...prefetched], ['/blog/Volatility-3/']);
        }
        await page.goto(`${origin}/blog/Volatility-3/`);
        await page.waitForFunction(() => window.audit.lcp > 0);
        assert.ok(await page.evaluate(() => window.audit.lcp <= 2500 && window.audit.cls <= 0.1));
        const images = await page.locator('.post img[src*="/volatility-3/"]').evaluateAll(images => images.map(img => [img.width, img.height, img.loading, img.getAttribute('src')]));
        assert.equal(images.length, 8);
        assert.ok(images.every(([width, height, loading, src]) => width > 0 && height > 0 && loading === 'lazy' && src.endsWith('.webp')));
        assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
        const toc = page.locator('.article-toc');
        await toc.locator('summary').click();
        const fragments = await toc.locator('a').evaluateAll(links => links.every(a => document.getElementById(decodeURIComponent(a.hash.slice(1)))));
        assert.ok(fragments);
        await page.goto(`${origin}/donate/`);
        const photo = page.locator('.donation-item img').first();
        await photo.evaluate(img => img.decode());
        const imageBytes = await photo.evaluate(img => performance.getEntriesByName(img.currentSrc)[0].encodedBodySize);
        assert.ok(imageBytes <= (mobile ? 25_000 : 70_000));
        assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
        const noJS = await browser.newContext({ javaScriptEnabled: false, viewport: page.viewportSize() });
        const accessible = await noJS.newPage();
        await accessible.goto(origin);
        await accessible.keyboard.press('Tab');
        assert.equal(await accessible.locator(':focus').getAttribute('href'), '#main-content');
        await accessible.keyboard.press('Enter');
        assert.equal(await accessible.locator(':focus').getAttribute('id'), 'main-content');
        if (mobile) await accessible.locator('.mobile-menu summary').press('Enter');
        const about = accessible.getByRole('navigation', { name: '主导航' }).getByRole('link', { name: '关于' });
        await about.click();
        assert.equal(accessible.url(), `${origin}/about/`);
        await noJS.close();
      }
      assert.deepEqual(failures, []);
      await context.close();
    }
  }
} finally {
  console.table(sample);
  await browser?.close();
  server.close();
}
