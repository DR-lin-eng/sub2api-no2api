import { readFileSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import ts from 'typescript'
import { describe, expect, it } from 'vitest'

const sourceRoot = resolve(__dirname, '../..')
const owner = 'core/networks/credentialEncryptionFallback.ts'

function runtimeFiles(directory: string, prefix = ''): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const relative = `${prefix}${entry.name}`
    if (entry.isDirectory()) {
      return entry.name === '__tests__' ? [] : runtimeFiles(resolve(directory, entry.name), `${relative}/`)
    }
    return /\.(ts|vue)$/.test(entry.name) && !/\.(spec|test)\./.test(entry.name) ? [relative] : []
  })
}

function memberPath(node: ts.Node): string | null {
  if (ts.isIdentifier(node)) return node.text
  if (!ts.isPropertyAccessExpression(node)) return null
  const parent = memberPath(node.expression)
  return parent ? `${parent}.${node.name.text}` : null
}

describe('credential crypto dependency scope', () => {
  it('confines node-forge imports to the credential encryption fallback', () => {
    const imports = runtimeFiles(sourceRoot).filter(path =>
      /['"]node-forge(?:\/[^'"]*)?['"]/.test(readFileSync(resolve(sourceRoot, path), 'utf8'))
    )
    expect(imports).toEqual([owner])
  })

  it('allows OAEP encryption and GCM but no signature verification', () => {
    // GHSA-86w9-cpqp-85rv affects PKCS#1 v1.5 signature verification.
    // The temporary audit exception is valid only while this scope holds.
    const source = ts.createSourceFile(owner, readFileSync(resolve(sourceRoot, owner), 'utf8'), ts.ScriptTarget.Latest, true)
    const allowedCalls = [
      'forge.pki.publicKeyFromAsn1', 'forge.asn1.fromDer', 'forge.md.sha256.create',
      'forge.cipher.createCipher', 'forge.util.createBuffer'
    ]
    const calls = new Set<string>()
    const schemes: string[] = []
    const visit = (node: ts.Node) => {
      if (ts.isPropertyAccessExpression(node)) {
        const path = memberPath(node)
        if (path?.startsWith('forge.')) {
          expect(allowedCalls.some(call => call === path || call.startsWith(`${path}.`)), path).toBe(true)
        }
        expect(node.name.text).not.toBe('verify')
      }
      if (ts.isElementAccessExpression(node)) {
        expect(memberPath(node.expression)).not.toMatch(/^(forge|publicKey)(\.|$)/)
      }
      if (ts.isCallExpression(node)) {
        const path = memberPath(node.expression)
        if (path?.startsWith('forge.')) calls.add(path)
        if (path === 'publicKey.encrypt' || path === 'forge.cipher.createCipher') {
          const scheme = node.arguments[path === 'publicKey.encrypt' ? 1 : 0]
          expect(scheme && ts.isStringLiteral(scheme)).toBe(true)
          if (scheme && ts.isStringLiteral(scheme)) schemes.push(scheme.text)
        }
      }
      ts.forEachChild(node, visit)
    }
    visit(source)
    expect([...calls].sort()).toEqual([...allowedCalls].sort())
    expect(schemes.sort()).toEqual(['AES-GCM', 'RSA-OAEP'])
  })
})
