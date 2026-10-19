// imagen del cargador (issue 146): subida en dataDir, kv y borrado
import { describe, it, expect, afterEach } from 'vitest'
import Database from 'better-sqlite3'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { initSchema, kvGet } from '../src/db.js'
import { _setForTests as _setInstallForTests, LEGACY_TOPOLOGY } from '../src/install.js'
import { normalizeExtensions, saveChargerImage, clearChargerImage, findChargerImage, _setForTests, GENERIC_EXTENSIONS } from '../src/extensions.js'

afterEach(() => {
  _setForTests(GENERIC_EXTENSIONS)
  _setInstallForTests(LEGACY_TOPOLOGY)
})

function setup() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'helios-img-'))
  const db = initSchema(new Database(':memory:'))
  const cfg = normalizeExtensions({ enabled: true, carCharger: { enabled: true, chargers: [{ key: 'main', name: 'Pared' }] } })
  db.prepare("INSERT INTO kv (key, value) VALUES ('extensions_config', ?)").run(JSON.stringify(cfg))
  return { db, dir }
}

const PNG = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='

describe('imagen del cargador', () => {
  it('guarda el fichero en dataDir y apunta imageUrl al endpoint', () => {
    const { db, dir } = setup()
    const res = saveChargerImage(db, dir, 'main', PNG)
    expect(res.ok).toBe(true)
    const img = findChargerImage(dir, 'main')
    expect(img?.mime).toBe('image/png')
    expect(fs.existsSync(img!.path)).toBe(true)
    const cfg = JSON.parse(kvGet(db, 'extensions_config')!)
    expect(cfg.carCharger.chargers[0].imageUrl).toBe('/api/charger/image/main')
  })

  it('rechaza formatos no soportados y datos vacios', () => {
    const { db, dir } = setup()
    expect(saveChargerImage(db, dir, 'main', 'data:text/html;base64,PGI+').ok).toBe(false)
    expect(saveChargerImage(db, dir, 'main', 'data:image/png;base64,').ok).toBe(false)
    expect(saveChargerImage(db, dir, 'main', null as unknown as string).ok).toBe(false)
    expect(findChargerImage(dir, 'main')).toBeNull()
  })

  it('reemplaza la extension anterior al subir otra', () => {
    const { db, dir } = setup()
    saveChargerImage(db, dir, 'main', PNG)
    const jpg = 'data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAABAAEBAREA/8QAFAABAAAAAAAAAAAAAAAAAAAACf/EABQQAQAAAAAAAAAAAAAAAAAAAAD/2gAIAQEAAD8AVN//2Q=='
    const res = saveChargerImage(db, dir, 'main', jpg)
    expect(res.ok).toBe(true)
    expect(findChargerImage(dir, 'main')?.mime).toBe('image/jpeg')
    expect(fs.readdirSync(path.join(dir, 'charger-images')).filter((f) => f.startsWith('main.'))).toEqual(['main.jpg'])
  })

  it('clear borra ficheros y deja imageUrl vacia', () => {
    const { db, dir } = setup()
    saveChargerImage(db, dir, 'main', PNG)
    expect(clearChargerImage(db, dir, 'main').ok).toBe(true)
    expect(findChargerImage(dir, 'main')).toBeNull()
    const cfg = JSON.parse(kvGet(db, 'extensions_config')!)
    expect(cfg.carCharger.chargers[0].imageUrl).toBe('')
  })
})
