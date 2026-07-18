import { webcrypto } from 'node:crypto'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import {
  clearCredentialKeyPrefetch,
  createCredentialEnvelope,
  prefetchCredentialKey
} from '../credentialEncryption'

function encodeBase64(value: ArrayBuffer): string {
  return Buffer.from(value).toString('base64').replace(/=+$/g, '')
}

function decodeBase64URL(value: string): Uint8Array {
  return new Uint8Array(Buffer.from(value, 'base64url'))
}

describe('credential encryption', () => {
  beforeEach(() => {
    clearCredentialKeyPrefetch()
    vi.restoreAllMocks()
    Object.defineProperty(globalThis, 'crypto', {
      configurable: true,
      value: webcrypto
    })
  })

  it('creates a server-decryptable RSA-OAEP and AES-GCM envelope', async () => {
    const serverTime = Math.floor(Date.now() / 1000) + 300
    const keyPair = await webcrypto.subtle.generateKey(
      {
        name: 'RSA-OAEP',
        modulusLength: 2048,
        publicExponent: new Uint8Array([1, 0, 1]),
        hash: 'SHA-256'
      },
      true,
      ['encrypt', 'decrypt']
    )
    const publicKey = await webcrypto.subtle.exportKey('spki', keyPair.publicKey)
    vi.spyOn(apiClient, 'get').mockResolvedValue({
      data: {
        algorithm: 'RSA-OAEP-256+A256GCM',
        key_id: 'test-key-id',
        public_key: encodeBase64(publicKey),
        expires_at: serverTime + 3600,
        flow_expires_at: serverTime + 900,
        server_time: serverTime
      }
    })

    const envelope = await createCredentialEnvelope('user@example.com', 'secret-123')
    const aesKeyBytes = await webcrypto.subtle.decrypt(
      { name: 'RSA-OAEP' },
      keyPair.privateKey,
      decodeBase64URL(envelope.encrypted_key)
    )
    const aesKey = await webcrypto.subtle.importKey(
      'raw',
      aesKeyBytes,
      { name: 'AES-GCM' },
      false,
      ['decrypt']
    )
    const plaintext = await webcrypto.subtle.decrypt(
      {
        name: 'AES-GCM',
        iv: decodeBase64URL(envelope.iv),
        additionalData: new TextEncoder().encode(envelope.key_id)
      },
      aesKey,
      decodeBase64URL(envelope.ciphertext)
    )
    const credentials = JSON.parse(new TextDecoder().decode(plaintext))

    expect(envelope.algorithm).toBe('RSA-OAEP-256+A256GCM')
    expect(credentials.email).toBe('user@example.com')
    expect(credentials.password).toBe('secret-123')
    expect(credentials.issued_at).toBeTypeOf('number')
    expect(Math.abs(credentials.issued_at - serverTime)).toBeLessThanOrEqual(1)
  })

  it('consumes a prefetched key once and fetches again for the next submission', async () => {
    const serverTime = Math.floor(Date.now() / 1000)
    const keyPair = await webcrypto.subtle.generateKey(
      {
        name: 'RSA-OAEP',
        modulusLength: 2048,
        publicExponent: new Uint8Array([1, 0, 1]),
        hash: 'SHA-256'
      },
      true,
      ['encrypt', 'decrypt']
    )
    const publicKey = await webcrypto.subtle.exportKey('spki', keyPair.publicKey)
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({
      data: {
        algorithm: 'RSA-OAEP-256+A256GCM',
        key_id: 'prefetched-key',
        public_key: encodeBase64(publicKey),
        expires_at: serverTime + 3600,
        flow_expires_at: serverTime + 900,
        server_time: serverTime
      }
    })

    await prefetchCredentialKey()
    await createCredentialEnvelope('first@example.com', 'secret-123')
    await createCredentialEnvelope('second@example.com', 'secret-456')

    expect(get).toHaveBeenCalledTimes(2)
  })

  it('refreshes a prefetched key that is too close to expiration', async () => {
    const serverTime = Math.floor(Date.now() / 1000)
    const keyPair = await webcrypto.subtle.generateKey(
      {
        name: 'RSA-OAEP',
        modulusLength: 2048,
        publicExponent: new Uint8Array([1, 0, 1]),
        hash: 'SHA-256'
      },
      true,
      ['encrypt', 'decrypt']
    )
    const publicKey = await webcrypto.subtle.exportKey('spki', keyPair.publicKey)
    const get = vi.spyOn(apiClient, 'get')
      .mockResolvedValueOnce({
        data: {
          algorithm: 'RSA-OAEP-256+A256GCM',
          key_id: 'nearly-expired-key',
          public_key: encodeBase64(publicKey),
          expires_at: serverTime + 3,
          flow_expires_at: serverTime + 3,
          server_time: serverTime
        }
      })
      .mockResolvedValueOnce({
        data: {
          algorithm: 'RSA-OAEP-256+A256GCM',
          key_id: 'fresh-key',
          public_key: encodeBase64(publicKey),
          expires_at: serverTime + 3600,
          flow_expires_at: serverTime + 900,
          server_time: serverTime
        }
      })

    await prefetchCredentialKey()
    const envelope = await createCredentialEnvelope('user@example.com', 'secret-123')

    expect(get).toHaveBeenCalledTimes(2)
    expect(envelope.key_id).toBe('fresh-key')
  })
})
