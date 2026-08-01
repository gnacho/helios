import { describe, it, expect, beforeEach } from 'vitest'
import Database from 'better-sqlite3'
import { initSchema, getUserByUsername } from '../src/db.js'
import { loginRateLimited, registerLoginFail, loginOk, registerUser, handleLogin, ensureBootstrapAdmin, changeOwnPassword } from '../src/auth.js'

function mockContext(ip, cookie) {
  const headers = new Headers()
  if (cookie) headers.set('cookie', cookie)
  return {
    req: {
      header: (name) => (name === 'x-forwarded-for' ? ip : undefined),
      url: 'http://localhost/',
      raw: { headers },
    },
    header: () => {},
    res: { headers: { append: () => {} } },
    env: {},
  }
}

let db

beforeEach(() => {
  db = initSchema(new Database(':memory:'))
})

describe('schema', () => {
  it('crea la tabla login_attempts (rate-limit)', () => {
    const tables = db.prepare("SELECT name FROM sqlite_master WHERE type='table'").all().map((r) => r.name)
    expect(tables).toContain('login_attempts')
    expect(tables).toContain('users')
    expect(tables).toContain('sessions')
  })
})

describe('registerUser', () => {
  it('crea usuario con hash bcrypt verificable', async () => {
    const u = await registerUser(db, 'demo', 'secreto123')
    expect(u.username).toBe('demo')
    const row = getUserByUsername(db, 'demo')
    expect(row.password_hash).not.toBe('secreto123')
  })

  it('rechaza duplicados', async () => {
    await registerUser(db, 'demo', 'secreto123')
    expect(await registerUser(db, 'demo', 'otra')).toBeNull()
  })
})

describe('handleLogin', () => {
  it('rechaza password incorrecta', async () => {
    await registerUser(db, 'demo', 'secreto123')
    expect(await handleLogin(db, mockContext('1.1.1.1'), { username: 'demo', password: 'mal' })).toBeNull()
  })

  it('acepta credenciales correctas y devuelve el usuario', async () => {
    await registerUser(db, 'demo', 'secreto123')
    const res = await handleLogin(db, mockContext('1.1.1.1'), { username: 'demo', password: 'secreto123' })
    expect(res.user.username).toBe('demo')
    expect(res.user.password_hash).toBeUndefined()
  })
})

describe('ensureBootstrapAdmin', () => {
  it('crea el admin desde .env en el primer arranque e idempotente', async () => {
    await ensureBootstrapAdmin(db) // AUTH_USER=admin / AUTH_PASS=test (script test)
    const admin = getUserByUsername(db, 'admin')
    expect(admin.role).toBe('admin')
    await ensureBootstrapAdmin(db)
    expect(db.prepare('SELECT COUNT(*) AS n FROM users').get().n).toBe(1)
    const res = await handleLogin(db, mockContext('1.1.1.1'), { username: 'admin', password: 'test' })
    expect(res.user.username).toBe('admin')
  })
})

describe('changeOwnPassword', () => {
  it('rechaza si la actual no coincide y acepta con la correcta', async () => {
    const u = await registerUser(db, 'demo', 'secreto123')
    expect((await changeOwnPassword(db, u.id, 'mal', 'nueva456')).reason).toBe('wrong_current')
    expect((await changeOwnPassword(db, u.id, 'secreto123', 'nueva456')).ok).toBe(true)
    expect(await handleLogin(db, mockContext('1.1.1.1'), { username: 'demo', password: 'secreto123' })).toBeNull()
    const res = await handleLogin(db, mockContext('1.1.1.1'), { username: 'demo', password: 'nueva456' })
    expect(res.user.username).toBe('demo')
  })

  it('cierra las demás sesiones del usuario salvo la actual', async () => {
    const u = await registerUser(db, 'demo', 'secreto123')
    const { createSession, getSession } = await import('../src/db.js')
    const s1 = createSession(db, u.id, 100000, 'ua1')
    const s2 = createSession(db, u.id, 100000, 'ua2')
    await changeOwnPassword(db, u.id, 'secreto123', 'nueva456', s1)
    expect(getSession(db, s1)).not.toBeNull()
    expect(getSession(db, s2)).toBeNull()
  })
})

describe('rate-limit de login (SQLite)', () => {
  it('bloquea tras 5 fallos y desbloquea con loginOk', () => {
    const c = mockContext('9.9.9.9')
    expect(loginRateLimited(db, c)).toBe(false)
    for (let i = 0; i < 5; i++) registerLoginFail(db, c)
    expect(loginRateLimited(db, c)).toBe(true)
    loginOk(db, c)
    expect(loginRateLimited(db, c)).toBe(false)
  })

  it('el bloqueo es por IP', () => {
    const a = mockContext('1.2.3.4')
    const b = mockContext('5.6.7.8')
    for (let i = 0; i < 5; i++) registerLoginFail(db, a)
    expect(loginRateLimited(db, a)).toBe(true)
    expect(loginRateLimited(db, b)).toBe(false)
  })
})
