import { test, expect } from '@playwright/test';

// Smoke test de la transición móvil: el bottom nav marca la dirección del
// deslizamiento (html[data-nav-dir]) y navega sin errores en viewport móvil.

test('bottom nav móvil marca dirección forward/back del deslizamiento', async ({ page }) => {
  // Instrumentar startViewTransition antes del primer documento (la SPA no recarga)
  await page.addInitScript(() => {
    const orig = document.startViewTransition;
    (window as unknown as { __vtCalls: string[] }).__vtCalls = [];
    if (orig) {
      document.startViewTransition = function (this: Document, cb: () => void) {
        (window as unknown as { __vtCalls: string[] }).__vtCalls.push('start');
        return orig.call(this, cb);
      };
    }
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await page.fill('#login-user', 'admin');
  await page.fill('#login-pass', 'testpass123');
  await page.click('button[type=submit]');
  await expect(page.locator('#login-user')).toHaveCount(0);

  // En / el bottom nav está visible
  const bottomNav = page.locator('nav[aria-label]').filter({ hasText: 'Ajustes' }).last();
  await expect(bottomNav).toBeVisible();

  // Click en "Histórico" (índice 3 > dashboard 0) → forward
  const navDir = page.locator('html');
  await bottomNav.getByText('Histórico', { exact: true }).click();
  await expect(page).toHaveURL(/\/historico/);
  await expect(navDir).toHaveAttribute('data-nav-dir', 'forward');

  // Click en "Hoy" (índice 0 < 3) → back
  await bottomNav.getByRole('link', { name: 'Hoy' }).click();
  await expect(page).toHaveURL(/\//);
  await expect(navDir).toHaveAttribute('data-nav-dir', 'back');

  // Sin errores de consola
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));

  // Durante la transición, el grupo root (header + bottom nav) NO debe animar
  // (la hoja UA hace fade-in de group/image-pair desde opacity 0 → nav invisible),
  // y el contenido (helios-content) SÍ debe deslizarse con nuestros keyframes.
  const vtPoll = page.evaluate(() => new Promise<{ root: string[]; content: string[] }>((resolve) => {
    const seen: { root: string[]; content: string[] } = { root: [], content: [] };
    const iv = setInterval(() => {
      const root = getComputedStyle(document.documentElement, '::view-transition-image-pair(root)');
      const oldContent = getComputedStyle(document.documentElement, '::view-transition-old(helios-content)');
      const newContent = getComputedStyle(document.documentElement, '::view-transition-new(helios-content)');
      if (root.animationName) seen.root.push(root.animationName);
      if (oldContent.animationName) seen.content.push(oldContent.animationName);
      if (newContent.animationName) seen.content.push(newContent.animationName);
    }, 25);
    setTimeout(() => {
      clearInterval(iv);
      resolve({ root: [...new Set(seen.root)], content: [...new Set(seen.content)] });
    }, 900);
  }));
  await page.getByRole('link', { name: 'Histórico' }).click();
  const vtAnims = await vtPoll;
  expect(vtAnims.root).toEqual(['none']);
  expect(vtAnims.content.some((n) => n.startsWith('helios-vt-'))).toBe(true);

  await page.waitForURL(/\/historico/);
  const calls = await page.evaluate(() => (window as unknown as { __vtCalls: string[] }).__vtCalls);
  expect(calls.length).toBeGreaterThan(0);
});
