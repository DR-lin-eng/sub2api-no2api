#!/usr/bin/env node
// Disposable Docker-backed API smoke test. Credentials exist only in a temporary
// env file and memory; never uses another deployment's database or containers.
import assert from 'node:assert/strict'
import crypto from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const image = process.argv[2] || 'sub2api-permission-groups:release-test'
const prefix = `permission-groups-smoke-${crypto.randomBytes(5).toString('hex')}`
const dir = mkdtempSync(join(tmpdir(), 'permission-groups-smoke-'))
const created = []
let networkCreated = false
let base = ''
let assertions = 0
const password = `T!${crypto.randomBytes(24).toString('base64url')}`
const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] }).trim()
const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms))
function run(name, args) {
  const container = `${prefix}-${name}`
  docker('run', '-d', '--name', container, '--network', prefix, ...args)
  created.push(container)
  return container
}
async function api(token, method, path, body, expected = 200) {
  const res = await fetch(base + '/api/v1' + path, {
    method, headers: { 'user-agent': 'permission-groups-smoke', 'content-type': 'application/json', ...(token ? { authorization: `Bearer ${token}` } : {}) },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
  const data = await res.json()
  assert.equal(res.status, expected, `${method} ${path}: ${data.message || ''}`)
  assertions++
  console.log(`${method} ${path} => ${res.status}`)
  return data.data
}
async function login(email) {
  const keyResponse = await fetch(base + '/api/v1/auth/credential-key', { headers: { 'user-agent': 'permission-groups-smoke' } })
  assert.equal(keyResponse.status, 200)
  const key = (await keyResponse.json()).data
  const cookie = keyResponse.headers.getSetCookie().map(value => value.split(';')[0]).join('; ')
  const aesKey = crypto.randomBytes(32)
  const iv = crypto.randomBytes(12)
  const cipher = crypto.createCipheriv('aes-256-gcm', aesKey, iv)
  cipher.setAAD(Buffer.from(key.key_id))
  const encrypted = Buffer.concat([cipher.update(JSON.stringify({ email, password, issued_at: key.server_time })), cipher.final(), cipher.getAuthTag()])
  const envelope = {
    algorithm: key.algorithm, key_id: key.key_id, iv: iv.toString('base64url'), ciphertext: encrypted.toString('base64url'),
    encrypted_key: crypto.publicEncrypt({ key: crypto.createPublicKey({ key: Buffer.from(key.public_key, 'base64'), format: 'der', type: 'spki' }), padding: crypto.constants.RSA_PKCS1_OAEP_PADDING, oaepHash: 'sha256' }, aesKey).toString('base64url'),
  }
  const res = await fetch(base + '/api/v1/auth/login', { method: 'POST', headers: { 'content-type': 'application/json', 'user-agent': 'permission-groups-smoke', cookie }, body: JSON.stringify({ credential_envelope: envelope }) })
  const data = await res.json()
  assert.equal(res.status, 200, `login ${email}: ${data.message || ''}`)
  assertions++
  console.log(`LOGIN ${email} => 200`)
  return data.data.access_token
}
async function acknowledge(token) {
  const status = await api(token, 'GET', '/admin/compliance')
  await api(token, 'POST', '/admin/compliance/accept', { phrase: status.ack_phrase_zh, language: 'zh' })
}
try {
  docker('network', 'create', prefix); networkCreated = true
  const env = join(dir, 'fixture.env')
  writeFileSync(env, `POSTGRES_USER=fixture\nPOSTGRES_PASSWORD=${password}\nPOSTGRES_DB=fixture\nPGDATA=/var/lib/postgresql/data\nAUTO_SETUP=true\nDATABASE_HOST=${prefix}-pg\nDATABASE_USER=fixture\nDATABASE_PASSWORD=${password}\nDATABASE_DBNAME=fixture\nDATABASE_SSLMODE=disable\nDATABASE_MAX_OPEN_CONNS=8\nDATABASE_MAX_IDLE_CONNS=2\nREDIS_HOST=${prefix}-redis\nREDIS_POOL_SIZE=8\nREDIS_MIN_IDLE_CONNS=1\nADMIN_EMAIL=admin@fixture.local\nADMIN_PASSWORD=${password}\nJWT_SECRET=${crypto.randomBytes(40).toString('hex')}\nSERVER_PORT=8080\n`, { mode: 0o600 })
  const pg = run('pg', ['--memory', '512m', '--tmpfs', '/var/lib/postgresql:rw,size=384m', '--env-file', env, 'postgres:18-alpine'])
  const redis = run('redis', ['--memory', '128m', 'redis:8-alpine', 'redis-server', '--save', '', '--appendonly', 'no', '--maxmemory', '64mb'])
  for (let i = 0; i < 90; i++) {
    try { docker('exec', pg, 'pg_isready', '-U', 'fixture'); docker('exec', redis, 'redis-cli', 'ping'); break } catch { if (i === 89) throw new Error('dependency startup timeout'); await sleep(1000) }
  }
  const app = run('app', ['--memory', '768m', '--tmpfs', '/app/data:rw,size=64m', '--env-file', env, '-p', '127.0.0.1::8080', image])
  const port = JSON.parse(docker('inspect', app))[0].NetworkSettings.Ports['8080/tcp'][0].HostPort
  base = `http://127.0.0.1:${port}`
  let ready = false
  for (let i = 0; i < 120; i++) {
    try { const res = await fetch(base + '/health'); if (res.ok) { ready = true; break } } catch { /* startup */ }
    await sleep(1000)
  }
  assert.ok(ready, 'fixture startup timeout')
  console.log('ISOLATED_APP_HEALTH => 200')
  const admin = await login('admin@fixture.local')
  await acknowledge(admin)
  const initial = await api(admin, 'GET', '/admin/settings/permission-groups')
  assert.equal(initial.groups[0].id, 'support')
  assert.deepEqual([...initial.groups[0].permissions].sort(), ['support.read', 'support.write', 'users.read_basic'])
  const groups = [...initial.groups, { id: 'readonly', name: '客服质检', permissions: ['support.read', 'users.read_basic'], built_in: false }, { id: 'settings_staff', name: '设置维护', permissions: ['settings.manage'], built_in: false }, { id: 'user_staff', name: '用户维护', permissions: ['users.manage'], built_in: false }]
  await api(admin, 'PUT', '/admin/settings/permission-groups', { groups })
  await api(admin, 'PUT', '/admin/settings/permission-groups', { groups: [] }, 400)
  const settings = await api(admin, 'GET', '/admin/settings')
  await api(admin, 'PUT', '/admin/settings', { ...settings, support_chat_enabled: true })
  const ordinary = await api(admin, 'POST', '/admin/users', { email: 'user@fixture.local', password, role: 'user' })
  const staff = await api(admin, 'POST', '/admin/users', { email: 'support@fixture.local', password, role: 'support' })
  await api(admin, 'POST', '/admin/users', { email: 'readonly@fixture.local', password, role: 'readonly' })
  await api(admin, 'POST', '/admin/users', { email: 'settings@fixture.local', password, role: 'settings_staff' })
  await api(admin, 'POST', '/admin/users', { email: 'users@fixture.local', password, role: 'user_staff' })
  const user = await login('user@fixture.local')
  const support = await login('support@fixture.local')
  await acknowledge(support)
  await api(user, 'POST', '/chat/messages', { content: 'permission group smoke', kind: 'text' })
  const inbox = await api(support, 'GET', '/admin/chat/conversations')
  const conversation = inbox.items?.find(c => (c.user_id ?? c.UserID) === ordinary.id) ?? inbox.items?.[0]
  assert.ok(conversation, `conversation missing: ${JSON.stringify(inbox)}`)
  console.log('CONVERSATION_FIXTURE', JSON.stringify(conversation))
  const profile = await api(support, 'GET', `/admin/users/${ordinary.id}/basic`)
  for (const field of ['api_keys', 'auth_identities', 'password', 'balance_notify_extra_emails']) assert.ok(!(field in profile), field)
  await api(support, 'POST', `/admin/chat/conversations/${conversation.id ?? conversation.ID}/messages`, { content: 'reply', kind: 'text' })
  await api(support, 'GET', `/admin/users/${staff.id}/basic`, undefined, 403)
  await api(support, 'GET', '/admin/users', undefined, 403)
  await api(support, 'GET', `/admin/users/${ordinary.id}/api-keys`, undefined, 403)
  await api(support, 'POST', `/admin/chat/conversations/${conversation.id ?? conversation.ID}/balance-transfers`, { amount: 1 }, 403)
  await api(support, 'GET', '/admin/settings', undefined, 403)
  await api(support, 'GET', '/admin/accounts', undefined, 403)
  await api(user, 'GET', '/admin/chat/conversations', undefined, 403)
  const readonly = await login('readonly@fixture.local'); await acknowledge(readonly)
  await api(readonly, 'GET', '/admin/chat/conversations')
  await api(readonly, 'POST', `/admin/chat/conversations/${conversation.id ?? conversation.ID}/messages`, { content: 'denied' }, 403)
  const settingsStaff = await login('settings@fixture.local'); await acknowledge(settingsStaff)
  await api(settingsStaff, 'PUT', '/admin/settings/permission-groups', { groups }, 403)
  await api(settingsStaff, 'POST', '/admin/settings/admin-api-keys', {}, 403)
  const usersStaff = await login('users@fixture.local'); await acknowledge(usersStaff)
  await api(usersStaff, 'PUT', `/admin/users/${ordinary.id}`, { notes: 'updated' })
  await api(usersStaff, 'PUT', `/admin/users/${ordinary.id}`, { balance: 10 }, 403)
  await api(usersStaff, 'PUT', `/admin/users/${ordinary.id}`, { password: 'not-applied' }, 403)
  await api(usersStaff, 'DELETE', `/admin/users/${staff.id}`, undefined, 403)
  await api(usersStaff, 'POST', '/admin/users', { email: 'denied@fixture.local', password, role: 'support' }, 403)
  const restricted = groups.map(g => g.id === 'support' ? { ...g, permissions: ['users.read_basic'] } : g)
  await api(admin, 'PUT', '/admin/settings/permission-groups', { groups: restricted })
  await api(support, 'GET', '/admin/chat/conversations', undefined, 403)
  await api(support, 'GET', `/admin/users/${ordinary.id}/basic`)
  console.log(`SMOKE_PASS assertions=${assertions}; role changes effective on existing JWT; basic profile allowlist verified`)
} finally {
  for (const name of created.reverse()) docker('rm', '-f', name)
  if (networkCreated) docker('network', 'rm', prefix)
  rmSync(dir, { recursive: true, force: true })
  console.log('CLEANUP_PASS isolated containers/network/temporary credentials removed')
}
