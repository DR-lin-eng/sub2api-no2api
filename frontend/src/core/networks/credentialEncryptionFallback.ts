import forge from 'node-forge'

interface CredentialFallbackInput {
  publicKeyDER: Uint8Array
  keyId: string
  plaintext: Uint8Array
  cryptoApi?: Pick<Crypto, 'getRandomValues'>
}

interface CredentialFallbackOutput {
  encryptedKey: Uint8Array
  iv: Uint8Array
  ciphertext: Uint8Array
}

function randomBytes(cryptoApi: CredentialFallbackInput['cryptoApi'], length: number): Uint8Array {
  if (!cryptoApi?.getRandomValues) {
    throw new Error('Secure credential encryption is not supported by this browser')
  }
  return cryptoApi.getRandomValues(new Uint8Array(length))
}

function bytesToBinary(bytes: Uint8Array): string {
  let result = ''
  for (const byte of bytes) {
    result += String.fromCharCode(byte)
  }
  return result
}

function binaryToBytes(value: string): Uint8Array {
  const result = new Uint8Array(value.length)
  for (let index = 0; index < value.length; index += 1) {
    result[index] = value.charCodeAt(index)
  }
  return result
}

export function encryptCredentialWithFallback({
  publicKeyDER,
  keyId,
  plaintext,
  cryptoApi
}: CredentialFallbackInput): CredentialFallbackOutput {
  const aesKey = randomBytes(cryptoApi, 32)
  const iv = randomBytes(cryptoApi, 12)
  const oaepSeed = randomBytes(cryptoApi, 32)
  try {
    const publicKey = forge.pki.publicKeyFromAsn1(forge.asn1.fromDer(bytesToBinary(publicKeyDER)))
    const encryptedKey = publicKey.encrypt(bytesToBinary(aesKey), 'RSA-OAEP', {
      md: forge.md.sha256.create(),
      mgf1: { md: forge.md.sha256.create() },
      seed: bytesToBinary(oaepSeed)
    })

    const cipher = forge.cipher.createCipher('AES-GCM', bytesToBinary(aesKey))
    cipher.start({
      iv: bytesToBinary(iv),
      additionalData: bytesToBinary(new TextEncoder().encode(keyId)),
      tagLength: 128
    })
    cipher.update(forge.util.createBuffer(bytesToBinary(plaintext), 'raw'))
    if (!cipher.finish()) {
      throw new Error('Credential encryption failed')
    }

    const ciphertext = cipher.output.getBytes() + cipher.mode.tag.getBytes()
    return {
      encryptedKey: binaryToBytes(encryptedKey),
      iv,
      ciphertext: binaryToBytes(ciphertext)
    }
  } finally {
    aesKey.fill(0)
    oaepSeed.fill(0)
  }
}
