import { test, expect } from '@playwright/test';
import type { APIRequestContext, Page } from '@playwright/test';

// Pagina del cargador con varios cargadores (issue 146): pestanas estilo
// Inversores, boton de encendido y selector de amperaje. HAOS es falso en el
// e2e (sin live real): se verifica la UI, no los datos.

const DOS = {
  enabled: true,
  carCharger: {
    enabled: true,
    chargers: [
      { key: 'main', name: 'Pared', powerId: 'sensor.a', powerUnit: 'W', energyTotalId: 'sensor.at', stateId: 'sensor.as', switchId: 'switch.a', ampId: 'number.a', chargingStates: ['charging'], connectedStates: ['charging'] , imageUrl: '/chargers/pared.png' },
      { key: 'c2', name: 'Portatil', powerId: 'sensor.b', powerUnit: 'kW', energyTotalId: 'sensor.bt', stateId: 'sensor.bs', switchId: 'switch.b', ampId: 'select.b', chargingStates: ['charging'], connectedStates: ['charging'] },
    ],
  },
};

const UNO = {
  enabled: true,
  carCharger: {
    enabled: true,
    chargers: [{ key: 'main', name: 'Pared', ampId: 'number.a' }],
  },
};

async function preparar(request: APIRequestContext, page: Page, config: object = DOS) {
  const login = await request.post('/api/auth/login', { data: { username: 'admin', password: 'testpass123' } });
  expect(login.ok()).toBeTruthy();
  const put = await request.put('/api/extensions', { data: config });
  expect(put.ok()).toBeTruthy();
  const cookie = login.headers()['set-cookie'].split(';')[0];
  const i = cookie.indexOf('=');
  await page.context().addCookies([{ name: cookie.slice(0, i), value: cookie.slice(i + 1), url: 'http://127.0.0.1:8199' }]);
}

test.describe('cargador: pestanas por cargador', () => {
  test.beforeEach(async ({ request, page }) => {
    await preparar(request, page);
  });

  test('dos pestanas con los nombres configurados', async ({ page }) => {
    await page.goto('/cargador');
    await expect(page.getByRole('tab', { name: 'Pared' })).toBeVisible({ timeout: 15000 });
    await expect(page.getByRole('tab', { name: 'Portatil' })).toBeVisible();

    // Click en la segunda pestana: URL y seleccion
    await page.getByRole('tab', { name: 'Portatil' }).click();
    await expect(page).toHaveURL(/tab=c2/);
    await expect(page.getByRole('tab', { name: 'Portatil' })).toHaveAttribute('aria-selected', 'true');
  });

  test('boton power y chips de amperaje, sin pageerrors', async ({ page }) => {
    const errores: string[] = [];
    page.on('pageerror', (e) => errores.push(String(e)));
    await page.goto('/cargador?tab=main');

    // Imagen del coche en lugar del icono (imageUrl configurada)
    await expect(page.getByRole('img', { name: 'Pared' })).toBeVisible({ timeout: 15000 });
    const power = page.getByRole('button', { name: /encender|apagar|turn on|turn off/i }).first();
    await expect(power).toBeVisible();
    // Sin HAOS real switchOn es undefined: el boton queda deshabilitado
    await expect(power).toBeDisabled();
    for (const a of [6, 8, 10, 13, 16, 20, 25, 32]) {
      await expect(page.getByRole('button', { name: String(a), exact: true })).toBeVisible();
    }
    expect(errores).toEqual([]);
  });

  test('con un solo cargador no hay pestanas', async ({ request, page }) => {
    await preparar(request, page, UNO);
    await page.goto('/cargador');
    // Sin tabs: la vista del unico cargador se renderiza directa (hero con
    // boton power) y solo queda el tablist del selector de periodo.
    await expect(page.getByRole('button', { name: /encender|apagar|turn on|turn off/i }).first()).toBeVisible({ timeout: 15000 });
    await expect(page.getByRole('tablist')).toHaveCount(1);
  });
});
