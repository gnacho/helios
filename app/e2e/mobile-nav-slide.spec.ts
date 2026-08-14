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

  await page.getByRole('link', { name: /Inversor/ }).click();
  await expect(page).toHaveURL(/\/inversores/);
  expect(errors).toEqual([]);
  await page.waitForTimeout(400);
  const calls = await page.evaluate(() => (window as unknown as { __vtCalls: string[] }).__vtCalls);
  expect(calls.length).toBeGreaterThan(0);
});
