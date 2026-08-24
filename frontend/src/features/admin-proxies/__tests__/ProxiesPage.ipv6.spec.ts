import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const source = readFileSync(
  resolve(process.cwd(), 'src/features/admin-proxies/presentation/pages/ProxiesPage.vue'),
  'utf8'
)

function extractRegex(): RegExp {
  const match = source.match(/const regex =\s*\n?\s*(\/\^\(https\?[^;\n]+\/i)\n/)
  expect(match, 'parseProxyUrl regex not found').toBeTruthy()
  return new RegExp((match as RegExpMatchArray)[1].slice(1, -2), 'i')
}

describe('proxy batch URL parsing (IPv6)', () => {
  it('accepts bracketed IPv6 and keeps ambiguous bare IPv6 invalid', () => {
    const regex = extractRegex()
    expect(regex.test('socks5://[2001:db8::1]:1080')).toBe(true)
    expect(regex.test('http://[::1]:8080')).toBe(true)
    expect(regex.test('socks5://user:pass@[2001:db8::1]:1080')).toBe(true)
    expect(regex.test('socks5://proxy.example.com:1080')).toBe(true)
    expect(regex.test('socks5://2001:db8::1:1080')).toBe(false)
    expect(regex.test('socks5://example.com:port')).toBe(false)
  })

  it('captures and unbrackets the IPv6 host', () => {
    const match = 'socks5://user:pass@[2001:db8::1]:1080'.match(extractRegex())
    expect(match).toBeTruthy()
    const [, , username, password, rawHost, port] = match as RegExpMatchArray
    expect(username).toBe('user')
    expect(password).toBe('pass')
    expect(rawHost.replace(/^\[|\]$/g, '')).toBe('2001:db8::1')
    expect(port).toBe('1080')
  })
})
