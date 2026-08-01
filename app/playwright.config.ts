import { defineConfig } from '@playwright/test';
import path from 'node:path';
import fs from 'node:fs';
import os from 'node:os';
import { fileURLToPath } from 'node:url';

const dirname = path.dirname(fileURLToPath(import.meta.url));
const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'helios-e2e-'));

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  retries: 0,
  use: { baseURL: 'http://127.0.0.1:8199' },
  webServer: {
    command: 'node ../server/src/index.js',
    url: 'http://127.0.0.1:8199/health',
    timeout: 20000,
    reuseExistingServer: !process.env.CI,
    env: {
      PORT: '8199',
      HOST: '127.0.0.1',
      HAOS_TOKEN: 'dummy-e2e',
      AUTH_USER: 'admin',
      AUTH_PASS: 'testpass123',
      DATA_DIR: dataDir,
      STATIC_DIR: path.join(dirname, 'dist'),
    },
  },
});
