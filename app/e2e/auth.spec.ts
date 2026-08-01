import { test, expect } from '@playwright/test';

// Contrato de sesión (auth.md): login → logout → debe quedarse en /login SIN recargar la SPA.

test('login incorrecto muestra error inline', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#login-user')).toBeVisible();
  await page.fill('#login-user', 'admin');
  await page.fill('#login-pass', 'incorrecta');
  await page.click('button[type=submit]');
  await expect(page.locator('[role=alert]')).toBeVisible();
});

test('login → dashboard → logout → vuelve al login', async ({ page }) => {
  await page.goto('/');
  await page.fill('#login-user', 'admin');
  await page.fill('#login-pass', 'testpass123');
  await page.click('button[type=submit]');

  // Tras login, el formulario desaparece (dashboard)
  await expect(page.locator('#login-user')).toHaveCount(0);

  // Logout desde Ajustes: la SPA NO se recarga, AuthGate muestra el login
  await page.goto('/ajustes');
  await page.getByRole('button', { name: /cerrar sesión|log out|退出登录/i }).click();
  await expect(page.locator('#login-user')).toBeVisible();
});
