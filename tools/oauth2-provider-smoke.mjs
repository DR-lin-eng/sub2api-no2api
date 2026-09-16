#!/usr/bin/env node
import assert from 'node:assert/strict'
import crypto from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const image = process.argv[2] || 'sub2api-oauth2-provider:test'
const prefix = `oauth2-provider-smoke-${crypto.randomBytes(5).toString('hex')}`
const dir = mkdtempSync(join(tmpdir(), 'oauth2-provider-smoke-'))
const created = []
const password = `T!${crypto.randomBytes(24).toString('base64url')}`
let networkCreated = false
let base = ''
let assertions = 0

const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] }).trim()
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

function run(name, args) {
  const container = `${prefix}-${name}`
  docker('run', '-d', '--name', container, '--network', prefix, ...args)
  created.push(container)
  return container
}

async function jsonRequest(method, path, { token, adminKey, body, form, basic, expected = 200 } = {}) {
  const headers = { 'user-agent': 'oauth2-provider-smoke' }
  if (token) headers.authorization = `Bearer ${token}`
  if (adminKey) headers['x-api-key'] = adminKey
  if (basic) headers.authorization = `Basic ${Buffer.from(`${basic.id}:${basic.secret}`).toString('base64')}`
  let encodedBody
  if (body !== undefined) {
    headers['content-type'] = 'application/json'
    encodedBody = JSON.stringify(body)
  } else if (form !== undefined) {
    headers['content-type'] = 'application/x-www-form-urlencoded'
    encodedBody = new URLSearchParams(form).toString()
  }
  const response = await fetch(base + path, { method, headers, ...(encodedBody === undefined ? {} : { body: encodedBody }) })
  const text = await response.text()
  const data = text ? JSON.parse(text) : null
  assert.equal(response.status, expected, `${method} ${path}: ${text}`)
  assertions++
  console.log(`${method} ${path.split('?', 1)[0]} => ${response.status}`)
  return { response, data }
}

async function api(token, method, path, body, expected = 200) {
  const { data } = await jsonRequest(method, `/api/v1${path}`, { token, body, expected })
  return data?.data
}

async function login(email) {
  const keyResponse = await fetch(base + '/api/v1/auth/credential-key', { headers: { 'user-agent': 'oauth2-provider-smoke' } })
  assert.equal(keyResponse.status, 200)
  const key = (await keyResponse.json()).data
  const cookie = keyResponse.headers.getSetCookie().map((value) => value.split(';')[0]).join('; ')
  const aesKey = crypto.randomBytes(32)
  const iv = crypto.randomBytes(12)
  const cipher = crypto.createCipheriv('aes-256-gcm', aesKey, iv)
  cipher.setAAD(Buffer.from(key.key_id))
  const encrypted = Buffer.concat([
    cipher.update(JSON.stringify({ email, password, issued_at: key.server_time })),
    cipher.final(),
    cipher.getAuthTag(),
  ])
  const envelope = {
    algorithm: key.algorithm,
    key_id: key.key_id,
    iv: iv.toString('base64url'),
    ciphertext: encrypted.toString('base64url'),
    encrypted_key: crypto.publicEncrypt({
      key: crypto.createPublicKey({ key: Buffer.from(key.public_key, 'base64'), format: 'der', type: 'spki' }),
      padding: crypto.constants.RSA_PKCS1_OAEP_PADDING,
      oaepHash: 'sha256',
    }, aesKey).toString('base64url'),
  }
  const response = await fetch(base + '/api/v1/auth/login', {
    method: 'POST',
    headers: { 'content-type': 'application/json', 'user-agent': 'oauth2-provider-smoke', cookie },
    body: JSON.stringify({ credential_envelope: envelope }),
  })
  const data = await response.json()
  assert.equal(response.status, 200, `login ${email}: ${JSON.stringify(data)}`)
  assertions++
  console.log(`LOGIN ${email} => 200`)
  return data.data.access_token
}

async function authorize(token, clientId, redirectURI, scopes, approved = true) {
  const verifier = crypto.randomBytes(48).toString('base64url')
  const challenge = crypto.createHash('sha256').update(verifier).digest('base64url')
  const request = {
    client_id: clientId,
    redirect_uri: redirectURI,
    response_type: 'code',
    scope: scopes,
    state: `state-${crypto.randomBytes(8).toString('hex')}`,
    code_challenge: challenge,
    code_challenge_method: 'S256',
  }
  const previewQuery = new URLSearchParams(request).toString()
  const preview = await api(token, 'GET', `/oauth2/authorize?${previewQuery}`)
  assert.equal(preview.client_id, clientId)
  const result = await api(token, 'POST', '/oauth2/authorize', { ...request, approved })
  const redirect = new URL(result.redirect_url)
  assert.equal(redirect.searchParams.get('state'), request.state)
  return { verifier, request, redirect }
}

async function exchange(client, grant) {
  const { data } = await jsonRequest('POST', '/oauth/token', {
    basic: { id: client.client_id, secret: client.client_secret },
    form: {
      grant_type: 'authorization_code',
      code: grant.redirect.searchParams.get('code'),
      redirect_uri: grant.request.redirect_uri,
      code_verifier: grant.verifier,
    },
  })
  return data
}

try {
  docker('network', 'create', prefix)
  networkCreated = true
  const env = join(dir, 'fixture.env')
  writeFileSync(env, [
    'POSTGRES_USER=fixture',
    `POSTGRES_PASSWORD=${password}`,
    'POSTGRES_DB=fixture',
    'PGDATA=/var/lib/postgresql/data',
    'AUTO_SETUP=true',
    `DATABASE_HOST=${prefix}-pg`,
    'DATABASE_USER=fixture',
    `DATABASE_PASSWORD=${password}`,
    'DATABASE_DBNAME=fixture',
    'DATABASE_SSLMODE=disable',
    'DATABASE_MAX_OPEN_CONNS=8',
    'DATABASE_MAX_IDLE_CONNS=2',
    `REDIS_HOST=${prefix}-redis`,
    'REDIS_POOL_SIZE=8',
    'REDIS_MIN_IDLE_CONNS=1',
    'ADMIN_EMAIL=admin@fixture.local',
    `ADMIN_PASSWORD=${password}`,
    `JWT_SECRET=${crypto.randomBytes(40).toString('hex')}`,
    'SERVER_PORT=8080',
    '',
  ].join('\n'), { mode: 0o600 })

  const postgres = run('pg', ['--memory', '512m', '--tmpfs', '/var/lib/postgresql:rw,size=384m', '--env-file', env, 'postgres:18-alpine'])
  const redis = run('redis', ['--memory', '128m', 'redis:8-alpine', 'redis-server', '--save', '', '--appendonly', 'no', '--maxmemory', '64mb'])
  for (let i = 0; i < 90; i++) {
    try {
      docker('exec', postgres, 'pg_isready', '-U', 'fixture')
      docker('exec', redis, 'redis-cli', 'ping')
      break
    } catch {
      if (i === 89) throw new Error('dependency startup timeout')
      await sleep(1000)
    }
  }

  const app = run('app', ['--memory', '768m', '--tmpfs', '/app/data:rw,size=64m', '--env-file', env, '-p', '127.0.0.1::8080', image])
  const port = JSON.parse(docker('inspect', app))[0].NetworkSettings.Ports['8080/tcp'][0].HostPort
  base = `http://127.0.0.1:${port}`
  let ready = false
  for (let i = 0; i < 120; i++) {
    try {
      const response = await fetch(base + '/health')
      if (response.ok) {
        ready = true
        break
      }
    } catch {
      // Startup probe.
    }
    await sleep(1000)
  }
  assert.ok(ready, 'fixture startup timeout')
  console.log('ISOLATED_APP_HEALTH => 200')

  const admin = await login('admin@fixture.local')
  const compliance = await api(admin, 'GET', '/admin/compliance')
  await api(admin, 'POST', '/admin/compliance/accept', { phrase: compliance.ack_phrase_zh, language: 'zh' })

  const initial = await api(admin, 'GET', '/admin/oauth2-provider')
  assert.equal(initial.enabled, false)
  await jsonRequest('GET', '/.well-known/oauth-authorization-server', { expected: 404 })

  const machineCredential = await api(admin, 'POST', '/admin/settings/admin-api-keys', {
    name: 'OAuth2 smoke machine credential',
    scopes: ['admin.settings.read', 'admin.settings.write'],
  }, 201)
  await jsonRequest('PUT', '/api/v1/admin/oauth2-provider', {
    adminKey: machineCredential.key,
    body: { enabled: true, issuer: base, access_token_ttl_seconds: 600 },
    expected: 403,
  })

  const configured = await api(admin, 'PUT', '/admin/oauth2-provider', {
    enabled: true,
    issuer: base,
    access_token_ttl_seconds: 600,
  })
  assert.equal(configured.enabled, true)

  const redirectURI = 'https://client.example.test/oauth/callback?fixed=1'
  const createdClient = await api(admin, 'POST', '/admin/oauth2-provider/clients', {
    name: 'OAuth2 smoke client',
    client_type: 'confidential',
    redirect_uris: [redirectURI],
    allowed_scopes: ['profile', 'email'],
    enabled: true,
  }, 201)
  assert.ok(createdClient.client.client_id)
  assert.ok(createdClient.client_secret)
  const client = { ...createdClient.client, client_secret: createdClient.client_secret }

  const discovery = await jsonRequest('GET', '/.well-known/oauth-authorization-server')
  assert.equal(discovery.data.authorization_endpoint, `${base}/oauth/authorize`)
  assert.equal(discovery.data.token_endpoint, `${base}/oauth/token`)

  const denied = await authorize(admin, client.client_id, redirectURI, 'profile', false)
  assert.equal(denied.redirect.searchParams.get('error'), 'access_denied')
  assert.equal(denied.redirect.searchParams.get('code'), null)

  const grant = await authorize(admin, client.client_id, redirectURI, 'profile email')
  assert.equal(grant.redirect.searchParams.get('fixed'), '1')
  const token = await exchange(client, grant)
  assert.equal(token.token_type, 'Bearer')
  assert.equal(token.expires_in, 600)
  assert.equal(token.scope, 'profile email')

  const replay = await jsonRequest('POST', '/oauth/token', {
    basic: { id: client.client_id, secret: client.client_secret },
    form: {
      grant_type: 'authorization_code',
      code: grant.redirect.searchParams.get('code'),
      redirect_uri: redirectURI,
      code_verifier: grant.verifier,
    },
    expected: 400,
  })
  assert.equal(replay.data.error, 'invalid_grant')

  const userinfo = await jsonRequest('GET', '/oauth/userinfo', { token: token.access_token })
  assert.ok(userinfo.data.sub)
  assert.equal(userinfo.data.email, 'admin@fixture.local')
  await jsonRequest('GET', '/api/v1/auth/me', { token: token.access_token, expected: 401 })
  await jsonRequest('GET', '/v1/models', { token: token.access_token, expected: 401 })

  await jsonRequest('POST', '/oauth/revoke', {
    basic: { id: client.client_id, secret: client.client_secret },
    form: { token: token.access_token },
  })
  const revoked = await jsonRequest('GET', '/oauth/userinfo', { token: token.access_token, expected: 401 })
  assert.equal(revoked.data.error, 'invalid_token')

  const scopedGrant = await authorize(admin, client.client_id, redirectURI, 'profile email')
  const scopedToken = await exchange(client, scopedGrant)
  await api(admin, 'PUT', `/admin/oauth2-provider/clients/${encodeURIComponent(client.client_id)}`, {
    allowed_scopes: ['profile'],
  })
  const scopeRevoked = await jsonRequest('GET', '/oauth/userinfo', { token: scopedToken.access_token, expected: 401 })
  assert.equal(scopeRevoked.data.error, 'invalid_token')

  await api(admin, 'PUT', '/admin/oauth2-provider', {
    enabled: false,
    issuer: base,
    access_token_ttl_seconds: 600,
  })
  await jsonRequest('GET', '/.well-known/oauth-authorization-server', { expected: 404 })

  console.log(`SMOKE_PASS assertions=${assertions}; admin-session control, authorize, PKCE, replay rejection, userinfo, scope revocation, token isolation and provider disable verified`)
} finally {
  for (const name of created.reverse()) docker('rm', '-f', name)
  if (networkCreated) docker('network', 'rm', prefix)
  rmSync(dir, { recursive: true, force: true })
  console.log('CLEANUP_PASS isolated containers/network/temporary credentials removed')
}
