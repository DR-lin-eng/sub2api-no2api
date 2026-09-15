import { apiClient } from './client'
import type { CredentialEnvelope } from '@/types'

const CREDENTIAL_ALGORITHM = 'RSA-OAEP-256+A256GCM' as const
const PUBLIC_KEY_EXPIRY_SKEW_SECONDS = 5

interface CredentialPublicKeyResponse {
  algorithm: typeof CREDENTIAL_ALGORITHM
  key_id: string
  public_key: string
  expires_at: number
  flow_expires_at: number
  server_time: number
}

interface PreparedCredentialKey {
  keyId: string
  publicKey: CryptoKey
  expiresAt: number
  serverTimeOffset: number
}

let prefetchedCredentialKey: Promise<PreparedCredentialKey> | null = null

function requireWebCrypto(): Crypto {
  const cryptoApi = globalThis.crypto
  if (!cryptoApi?.subtle) {
    throw new Error('Secure credential encryption is not supported by this browser')
  }
  return cryptoApi
}

function decodeBase64(value: string): Uint8Array {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized.padEnd(normalized.length + ((4 - (normalized.length % 4)) % 4), '=')
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }
  return bytes
}

function encodeBase64URL(value: ArrayBuffer): string {
  const bytes = new Uint8Array(value)
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

async function fetchCredentialKey(): Promise<PreparedCredentialKey> {
  const now = Math.floor(Date.now() / 1000)
  const { data } = await apiClient.get<CredentialPublicKeyResponse>('/auth/credential-key')
  if (data.algorithm !== CREDENTIAL_ALGORITHM) {
    throw new Error('Unsupported credential encryption algorithm')
  }
  if (data.expires_at <= data.server_time || data.flow_expires_at <= data.server_time) {
    throw new Error('Credential encryption key is expired')
  }

  const cryptoApi = requireWebCrypto()
  const publicKey = await cryptoApi.subtle.importKey(
    'spki',
    decodeBase64(data.public_key),
    { name: 'RSA-OAEP', hash: 'SHA-256' },
    false,
    ['encrypt']
  )
  return {
    keyId: data.key_id,
    publicKey,
    expiresAt: now + Math.max(0, Math.min(data.expires_at, data.flow_expires_at) - data.server_time),
    serverTimeOffset: data.server_time - now
  }
}

async function takeCredentialKey(): Promise<PreparedCredentialKey> {
  const prepared = prefetchedCredentialKey
  if (prepared) {
    prefetchedCredentialKey = null
  }

  let key: PreparedCredentialKey
  try {
    key = await (prepared || fetchCredentialKey())
  } catch (error) {
    if (!prepared) {
      throw error
    }
    key = await fetchCredentialKey()
  }

  const now = Math.floor(Date.now() / 1000)
  if (key.expiresAt <= now + PUBLIC_KEY_EXPIRY_SKEW_SECONDS) {
    return fetchCredentialKey()
  }
  return key
}

// Starts a one-shot key request when an auth view opens. The result is consumed
// by the next credential submission and is never persisted across page loads.
export function prefetchCredentialKey(): Promise<void> {
  if (!prefetchedCredentialKey) {
    const request = fetchCredentialKey()
    prefetchedCredentialKey = request
    void request.catch(() => {
      if (prefetchedCredentialKey === request) {
        prefetchedCredentialKey = null
      }
    })
  }
  return prefetchedCredentialKey.then(
    () => undefined,
    () => undefined
  )
}

export async function createCredentialEnvelope(email: string, password: string): Promise<CredentialEnvelope> {
  const cryptoApi = requireWebCrypto()
  const serverKey = await takeCredentialKey()
  const aesKey = await cryptoApi.subtle.generateKey(
    { name: 'AES-GCM', length: 256 },
    true,
    ['encrypt']
  )
  const rawAESKey = await cryptoApi.subtle.exportKey('raw', aesKey)
  const encryptedKey = await cryptoApi.subtle.encrypt(
    { name: 'RSA-OAEP' },
    serverKey.publicKey,
    rawAESKey
  )

  const iv = cryptoApi.getRandomValues(new Uint8Array(12))
  const encoder = new TextEncoder()
  const plaintext = encoder.encode(JSON.stringify({
    email,
    password,
    issued_at: Math.floor(Date.now() / 1000) + serverKey.serverTimeOffset
  }))
  const ciphertext = await cryptoApi.subtle.encrypt(
    {
      name: 'AES-GCM',
      iv,
      additionalData: encoder.encode(serverKey.keyId)
    },
    aesKey,
    plaintext
  )

  return {
    algorithm: CREDENTIAL_ALGORITHM,
    key_id: serverKey.keyId,
    encrypted_key: encodeBase64URL(encryptedKey),
    iv: encodeBase64URL(iv.buffer),
    ciphertext: encodeBase64URL(ciphertext)
  }
}

export function clearCredentialKeyPrefetch(): void {
  prefetchedCredentialKey = null
}
