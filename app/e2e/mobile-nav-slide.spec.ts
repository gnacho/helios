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

  // En / el bottom nav está visible (clase md:hidden; ya no contiene Ajustes)
  const bottomNav = page.locator('nav.md\\:hidden');
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

  // Durante la transición, los grupos del shell (root + header + nav) NO deben
  // animar (la hoja UA hace fade-in desde opacity 0 → shell invisible), y el
  // contenido (helios-content) SÍ debe deslizarse con nuestros keyframes.
  const vtPoll = page.evaluate(() => new Promise<Record<string, string[]>>((resolve) => {
    const seen: Record<string, string[]> = {};
    const names = ['root', 'helios-header', 'helios-nav', 'helios-content'];
    const iv = setInterval(() => {
      for (const n of names) {
        for (const pseudo of ['old', 'new']) {
          const cs = getComputedStyle(document.documentElement, `::view-transition-${pseudo}(${n})`);
          if (cs.animationName && !seen[`${n}:${pseudo}`]) {
            seen[`${n}:${pseudo}`] = [cs.animationName];
          }
        }
      }
    }, 25);
    setTimeout(() => {
      clearInterval(iv);
      resolve(seen);
    }, 900);
  }));
  await page.getByRole('link', { name: 'Histórico' }).click();
  const vtAnims = await vtPoll;
  console.log('vtAnims:', JSON.stringify(vtAnims, null, 1));
  for (const key of ['root', 'helios-header', 'helios-nav']) {
    for (const pseudo of ['old', 'new']) {
      const a = vtAnims[`${key}:${pseudo}`];
      expect(a).toBeDefined();
      expect(a![0]).toBe('none');
    }
  }
  expect(vtAnims['helios-content:old']?.some((n) => n.startsWith('helios-vt-'))).toBe(true);
  expect(vtAnims['helios-content:new']?.some((n) => n.startsWith('helios-vt-'))).toBe(true);

  await page.waitForURL(/\/historico/);
  const calls = await page.evaluate(() => (window as unknown as { __vtCalls: string[] }).__vtCalls);
  expect(calls.length).toBeGreaterThan(0);
});

test('avatar del header móvil enlaza a Ajustes y el bottom nav ya no lo incluye', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await page.fill('#login-user', 'admin');
  await page.fill('#login-pass', 'testpass123');
  await page.click('button[type=submit]');
  await expect(page.locator('#login-user')).toHaveCount(0);

  // El bottom nav no tiene Ajustes
  const bottomNav = page.locator('nav.md\\:hidden');
  await expect(bottomNav).toBeVisible();
  await expect(bottomNav.getByRole('link', { name: 'Ajustes' })).toHaveCount(0);

  // El avatar del header móvil enlaza a Ajustes (solo avatar, sin nombre <sm)
  const avatar = page.locator('header.md\\:hidden').getByRole('link', { name: 'Ajustes' });
  await expect(avatar).toBeVisible();
  await expect(avatar.getByText('admin')).toBeHidden();
  await avatar.click();
  await expect(page).toHaveURL(/\/ajustes/);
});
