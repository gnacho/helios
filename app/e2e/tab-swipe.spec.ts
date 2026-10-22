import { test, expect } from '@playwright/test';
import type { APIRequestContext, Page, CDPSession } from '@playwright/test';

// Swipe horizontal entre pestanas en movil (issue 154), en Inversores y en
// Cargador. Los gestos se emiten con eventos tactiles reales via CDP, igual que
// un dedo: `page.touchscreen` solo hace toques sueltos, no arrastres.

const VIEWPORT = { width: 390, height: 844 };

const TOPOLOGIA = {
  topology: {
    inverters: [
      { key: 'inv1', name: 'Solis', kwp: 4.4 },
      { key: 'inv2', name: 'Fox', kwp: 2.7 },
    ],
  },
};

const CARGA_DOS = {
  enabled: true,
  carCharger: {
    enabled: true,
    chargers: [
      { key: 'main', name: 'Pared' },
      { key: 'c2', name: 'Portatil' },
    ],
  },
};

async function sesion(request: APIRequestContext, page: Page) {
  const login = await request.post('/api/auth/login', { data: { username: 'admin', password: 'testpass123' } });
  expect(login.ok()).toBeTruthy();
  const cookie = login.headers()['set-cookie'].split(';')[0];
  const i = cookie.indexOf('=');
  await page.context().addCookies([{ name: cookie.slice(0, i), value: cookie.slice(i + 1), url: 'http://127.0.0.1:8199' }]);
}

/** Arrastre horizontal con dedo unico, en varios pasos (como un gesto real). */
async function deslizar(cdp: CDPSession, de: number, a: number, y: number) {
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: de, y }],
  });
  const pasos = 10;
  for (let p = 1; p <= pasos; p++) {
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: de + ((a - de) * p) / pasos, y }],
    });
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
}

test.describe('swipe entre pestanas en movil', () => {
  test.use({ viewport: VIEWPORT, hasTouch: true, isMobile: true });

  test('inversores: mientras se arrastra, la pestana vecina va al lado', async ({ request, page }) => {
    await sesion(request, page);
    await request.put('/api/config', { data: TOPOLOGIA });
    await page.goto('/inversores');
    await expect(page.getByRole('tab', { name: /Solis/ })).toBeVisible({ timeout: 15000 });

    const cdp = await page.context().newCDPSession(page);
    const peek = page.locator('[data-peek="1"]');
    await expect(peek).toHaveCount(0);

    // Arrastre a medias (sin soltar): la vecina se monta AL LADO y el track se mueve
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: 330, y: 500 }] });
    for (let p = 1; p <= 8; p++) {
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: 330 - p * 18, y: 500 }] });
    }
    await expect(peek).toHaveCount(1);

    const estado = await peek.evaluate((el) => {
      const track = el.parentElement as HTMLElement;
      const activa = track.children[0] as HTMLElement;
      const vecina = el as HTMLElement;
      return {
        transform: track.style.transform,
        // La vecina arranca justo donde acaba la activa: van pegadas, no superpuestas.
        gap: Math.round(vecina.getBoundingClientRect().left - activa.getBoundingClientRect().right),
        anchoActiva: Math.round(activa.getBoundingClientRect().width),
      };
    });
    expect(estado.anchoActiva).toBeGreaterThan(300);
    expect(Math.abs(estado.gap)).toBeLessThanOrEqual(1);
    expect(estado.transform).toMatch(/translate3d\(-\d+px/);
    // Aun no ha cambiado de pestana: solo se esta previsualizando
    await expect(page).toHaveURL(/\/inversores$/);

    // Se suelta: el vecino desaparece y la pestana cambia
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await expect(page).toHaveURL(/tab=inv1/, { timeout: 5000 });
    await expect(peek).toHaveCount(0);
  });

  test('inversores: deslizar cambia de pestana en ambos sentidos', async ({ request, page }) => {
    await sesion(request, page);
    const put = await request.put('/api/config', { data: TOPOLOGIA });
    expect(put.ok()).toBeTruthy();

    await page.goto('/inversores');
    await expect(page.getByRole('tab', { name: /Solis/ })).toBeVisible({ timeout: 15000 });

    const cdp = await page.context().newCDPSession(page);
    const y = 500;

    // Izquierda desde "Comparativa" -> primer inversor
    await deslizar(cdp, 320, 60, y);
    await expect(page).toHaveURL(/tab=inv1/, { timeout: 5000 });
    await page.waitForTimeout(500);

    // Izquierda otra vez -> segundo inversor
    await deslizar(cdp, 320, 60, y);
    await expect(page).toHaveURL(/tab=inv2/, { timeout: 5000 });
    await expect(page.getByRole('tab', { name: /Fox/ })).toHaveAttribute('aria-selected', 'true');
    await page.waitForTimeout(500);

    // Derecha -> vuelve al primero
    await deslizar(cdp, 60, 320, y);
    await expect(page).toHaveURL(/tab=inv1/, { timeout: 5000 });
    await page.waitForTimeout(500);

    // Y otra derecha -> comparativa
    await deslizar(cdp, 60, 320, y);
    await expect(page).toHaveURL(/tab=compare/, { timeout: 5000 });
  });

  test('inversores: un arrastre corto no cambia de pestana', async ({ request, page }) => {
    await sesion(request, page);
    await request.put('/api/config', { data: TOPOLOGIA });
    await page.goto('/inversores?tab=inv1');
    await expect(page.getByRole('tab', { name: /Solis/ })).toHaveAttribute('aria-selected', 'true', { timeout: 15000 });

    const cdp = await page.context().newCDPSession(page);
    await deslizar(cdp, 220, 190, 700); // 30 px: por debajo del umbral de encaje
    await expect(page).toHaveURL(/tab=inv1/);
  });

  test('cargador: deslizar cambia de cargador', async ({ request, page }) => {
    await sesion(request, page);
    const put = await request.put('/api/extensions', { data: CARGA_DOS });
    expect(put.ok()).toBeTruthy();

    await page.goto('/cargador');
    await expect(page.getByRole('tab', { name: 'Pared' })).toBeVisible({ timeout: 15000 });

    const cdp = await page.context().newCDPSession(page);
    await deslizar(cdp, 320, 60, 700);
    await expect(page).toHaveURL(/tab=c2/, { timeout: 5000 });
    await expect(page.getByRole('tab', { name: 'Portatil' })).toHaveAttribute('aria-selected', 'true');

    await deslizar(cdp, 60, 320, 700);
    await expect(page).toHaveURL(/tab=main/, { timeout: 5000 });
  });

  test('cargador: el gesto vertical sigue haciendo scroll', async ({ request, page }) => {
    await sesion(request, page);
    await request.put('/api/extensions', { data: CARGA_DOS });
    await page.goto('/cargador');
    await expect(page.getByRole('tab', { name: 'Pared' })).toBeVisible({ timeout: 15000 });

    const cdp = await page.context().newCDPSession(page);
    // Arrastre claramente vertical: debe desplazar la pagina, sin cambiar de pestana
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: 200, y: 700 }] });
    for (let p = 1; p <= 10; p++) {
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: 200, y: 700 - p * 20 }] });
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });

    await expect(page).toHaveURL(/\/cargador$/);
    const y = await page.evaluate(() => window.scrollY);
    expect(y).toBeGreaterThan(0);
  });
});
