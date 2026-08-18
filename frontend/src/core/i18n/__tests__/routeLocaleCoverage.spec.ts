import { existsSync, readFileSync, statSync } from 'node:fs'
import { dirname, extname, relative, resolve } from 'node:path'
import ts from 'typescript'
import { describe, expect, it } from 'vitest'

import { getLocaleScopesForRoute, type LocaleScope } from '@/core/i18n'
import enAdmin from '@/core/i18n/locales/en/admin'
import enBatchImage from '@/core/i18n/locales/en/batchImage'
import enCommon from '@/core/i18n/locales/en/common'
import enDashboard from '@/core/i18n/locales/en/dashboard'
import enLanding from '@/core/i18n/locales/en/landing'
import enMediaStudio from '@/core/i18n/locales/en/mediaStudio'
import enMisc from '@/core/i18n/locales/en/misc'
import enSupportChat from '@/core/i18n/locales/en/supportChat'
import zhAdmin from '@/core/i18n/locales/zh/admin'
import zhBatchImage from '@/core/i18n/locales/zh/batchImage'
import zhCommon from '@/core/i18n/locales/zh/common'
import zhDashboard from '@/core/i18n/locales/zh/dashboard'
import zhLanding from '@/core/i18n/locales/zh/landing'
import zhMediaStudio from '@/core/i18n/locales/zh/mediaStudio'
import zhMisc from '@/core/i18n/locales/zh/misc'
import zhSupportChat from '@/core/i18n/locales/zh/supportChat'

type Messages = Record<string, unknown>

interface RouteSource {
  path: string
  component: string
  metaKeys: string[]
}

interface SourceImport {
  specifier: string
  names?: string[]
}

const srcRoot = resolve(process.cwd(), 'src')
const routesFile = resolve(srcRoot, 'core/routes/index.ts')
const appFile = resolve(srcRoot, 'App.vue')
const scriptContentsCache = new Map<string, string[]>()
const sourceImportsCache = new Map<string, SourceImport[]>()
const sourceCandidateKeysCache = new Map<string, Set<string>>()

const localeScopes = {
  en: {
    base: { ...enLanding, ...enCommon },
    user: { ...enDashboard, ...enMisc },
    batchImage: enBatchImage,
    mediaStudio: enMediaStudio,
    supportChat: enSupportChat,
    admin: { admin: enAdmin },
  },
  zh: {
    base: { ...zhLanding, ...zhCommon },
    user: { ...zhDashboard, ...zhMisc },
    batchImage: zhBatchImage,
    mediaStudio: zhMediaStudio,
    supportChat: zhSupportChat,
    admin: { admin: zhAdmin },
  },
} satisfies Record<'en' | 'zh', Record<LocaleScope, Messages>>

const fullMessages = {
  en: mergeMessages(Object.values(localeScopes.en)),
  zh: mergeMessages(Object.values(localeScopes.zh)),
}

const knownNamespaces = new Set([
  ...Object.keys(fullMessages.en),
  ...Object.keys(fullMessages.zh),
])

function mergeMessages(parts: readonly Messages[]): Messages {
  const merged: Messages = {}
  for (const part of parts) {
    for (const [key, value] of Object.entries(part)) {
      if (isRecord(merged[key]) && isRecord(value)) {
        merged[key] = mergeMessages([merged[key] as Messages, value])
      } else {
        merged[key] = value
      }
    }
  }
  return merged
}

function isRecord(value: unknown): value is Messages {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function resolveMessage(messages: Messages, key: string): unknown {
  return key.split('.').reduce<unknown>((current, segment) => {
    if (!isRecord(current)) return undefined
    return current[segment]
  }, messages)
}

function flattenLeafKeys(value: unknown, prefix = '', out: string[] = []): string[] {
  if (!isRecord(value)) {
    if (prefix) out.push(prefix)
    return out
  }
  for (const [key, child] of Object.entries(value)) {
    flattenLeafKeys(child, prefix ? `${prefix}.${key}` : key, out)
  }
  return out
}

function parseRouteSources(): RouteSource[] {
  const source = readFileSync(routesFile, 'utf8')
  const lines = source.split('\n')
  const routes: RouteSource[] = []
  let currentPath: string | undefined
  let currentComponent: string | undefined
  let currentMetaKeys: string[] = []

  const flush = () => {
    if (currentPath && currentComponent) {
      routes.push({ path: currentPath, component: currentComponent, metaKeys: currentMetaKeys })
    }
    currentPath = undefined
    currentComponent = undefined
    currentMetaKeys = []
  }

  for (const line of lines) {
    const pathMatch = line.match(/^\s{4}path:\s*['"]([^'"]+)['"]/)
    if (pathMatch) {
      flush()
      currentPath = pathMatch[1]
      continue
    }

    const componentMatch = line.match(/component:\s*\(\)\s*=>\s*import\(['"]([^'"]+)['"]\)/)
    if (componentMatch) currentComponent = componentMatch[1]

    const metaKeyMatch = line.match(/(?:titleKey|descriptionKey):\s*['"]([^'"]+)['"]/)
    if (metaKeyMatch) currentMetaKeys.push(metaKeyMatch[1])
  }
  flush()
  return routes
}

function resolveSourceImport(fromFile: string, specifier: string): string | undefined {
  let base: string
  if (specifier.startsWith('@/')) {
    base = resolve(srcRoot, specifier.slice(2))
  } else if (specifier.startsWith('.')) {
    base = resolve(dirname(fromFile), specifier)
  } else {
    return undefined
  }

  const candidates = extname(base)
    ? [base]
    : [base, `${base}.ts`, `${base}.vue`, resolve(base, 'index.ts')]
  return candidates.find((candidate) => existsSync(candidate) && statSync(candidate).isFile())
}

function scriptContents(file: string): string[] {
  const cached = scriptContentsCache.get(file)
  if (cached) return cached
  const source = readFileSync(file, 'utf8')
  const contents = file.endsWith('.vue')
    ? [...source.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)].map((match) => match[1])
    : [source]
  scriptContentsCache.set(file, contents)
  return contents
}

function importedNames(node: ts.ImportDeclaration): string[] | undefined {
  const clause = node.importClause
  if (!clause) return undefined
  const names: string[] = []
  if (clause.name) names.push('default')
  if (clause.namedBindings) {
    if (ts.isNamespaceImport(clause.namedBindings)) return undefined
    for (const element of clause.namedBindings.elements) {
      names.push(element.propertyName?.text ?? element.name.text)
    }
  }
  return names
}

function sourceImports(file: string, requestedExports?: ReadonlySet<string>): SourceImport[] {
  const cacheKey = `${file}\0${requestedExports ? [...requestedExports].sort().join(',') : '*'}`
  const cached = sourceImportsCache.get(cacheKey)
  if (cached) return cached
  const imports: SourceImport[] = []
  for (const source of scriptContents(file)) {
    const ast = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
    ast.forEachChild(function visit(node) {
      if (ts.isImportDeclaration(node) && ts.isStringLiteral(node.moduleSpecifier)) {
        imports.push({ specifier: node.moduleSpecifier.text, names: importedNames(node) })
      }
      if (
        ts.isExportDeclaration(node) &&
        node.moduleSpecifier &&
        ts.isStringLiteral(node.moduleSpecifier)
      ) {
        if (node.exportClause && ts.isNamedExports(node.exportClause)) {
          const selected = node.exportClause.elements.filter(
            (element) => !requestedExports || requestedExports.has(element.name.text),
          )
          if (selected.length > 0) {
            imports.push({
              specifier: node.moduleSpecifier.text,
              names: selected.map((element) => element.propertyName?.text ?? element.name.text),
            })
          }
        } else {
          imports.push({
            specifier: node.moduleSpecifier.text,
            names: requestedExports ? [...requestedExports] : undefined,
          })
        }
      }
      ts.forEachChild(node, visit)
    })
  }
  sourceImportsCache.set(cacheKey, imports)
  return imports
}

function dependencyClosure(rootFiles: readonly string[]): Set<string> {
  const visited = new Set<string>()
  const processed = new Set<string>()
  const pending = rootFiles.map((file) => ({ file, names: undefined as string[] | undefined }))
  while (pending.length > 0) {
    const next = pending.pop()
    if (!next) continue
    const { file, names } = next
    const processKey = `${file}\0${names ? [...names].sort().join(',') : '*'}`
    if (processed.has(processKey)) continue
    processed.add(processKey)
    visited.add(file)
    const requestedExports = names ? new Set(names) : undefined
    for (const sourceImport of sourceImports(file, requestedExports)) {
      const dependency = resolveSourceImport(file, sourceImport.specifier)
      if (dependency) pending.push({ file: dependency, names: sourceImport.names })
    }
  }
  return visited
}

function isKnownLocaleKey(value: string): boolean {
  return (
    resolveMessage(fullMessages.en, value) !== undefined ||
    resolveMessage(fullMessages.zh, value) !== undefined
  )
}

function addCandidateKey(value: string, keys: Set<string>, force = false): void {
  const namespace = value.split('.', 1)[0]
  if (
    value.includes('.') &&
    (force || (knownNamespaces.has(namespace) && isKnownLocaleKey(value)))
  ) keys.add(value)
}

function propertyName(node: ts.PropertyName): string | undefined {
  if (ts.isIdentifier(node) || ts.isStringLiteral(node)) return node.text
  return undefined
}

function isTranslationCallLiteral(node: ts.StringLiteral | ts.NoSubstitutionTemplateLiteral): boolean {
  const parent = node.parent
  if (ts.isCallExpression(parent) && parent.arguments[0] === node) {
    const callee = parent.expression
    const name = ts.isIdentifier(callee)
      ? callee.text
      : ts.isPropertyAccessExpression(callee)
        ? callee.name.text
        : ''
    return ['t', '$t', 'translate', 'translateText', 'interpolate'].includes(name)
  }
  return false
}

function isLocaleKeyProperty(node: ts.StringLiteral | ts.NoSubstitutionTemplateLiteral): boolean {
  const parent = node.parent
  if (ts.isPropertyAssignment(parent) && parent.initializer === node) {
    const name = propertyName(parent.name)
    return Boolean(name && /(?:^|_)(?:title|description|label|message|hint|error)Key$/i.test(name))
  }
  return false
}

function candidateKeysFromScript(
  source: string,
  file: string,
  keys: Set<string>,
  directOnly: boolean,
): void {
  const ast = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  ast.forEachChild(function visit(node) {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      const direct = isTranslationCallLiteral(node)
      if (direct || !directOnly) {
        addCandidateKey(node.text, keys, direct || isLocaleKeyProperty(node))
      }
    }
    ts.forEachChild(node, visit)
  })
}

function sourceCandidateKeys(file: string, directOnly = false): Set<string> {
  const cacheKey = `${file}\0${directOnly}`
  const cached = sourceCandidateKeysCache.get(cacheKey)
  if (cached) return cached
  const keys = new Set<string>()
  const source = readFileSync(file, 'utf8')
  for (const script of scriptContents(file)) {
    candidateKeysFromScript(script, file, keys, directOnly)
  }

  if (file.endsWith('.vue')) {
    const template = source
      .replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, '')
      .replace(/<style\b[^>]*>[\s\S]*?<\/style>/g, '')
      .replace(/<!--[\s\S]*?-->/g, '')
    if (!directOnly) {
      for (const match of template.matchAll(/(['"])([A-Za-z][A-Za-z0-9_-]*(?:\.[A-Za-z0-9_-]+)+)\1/g)) {
        addCandidateKey(match[2], keys)
      }
    }
    for (const match of template.matchAll(/(?:\bt|\$t)\(\s*(['"])([A-Za-z][A-Za-z0-9_-]*(?:\.[A-Za-z0-9_-]+)+)\1/g)) {
      addCandidateKey(match[2], keys, true)
    }
  }
  sourceCandidateKeysCache.set(cacheKey, keys)
  return keys
}

function runtimeSourceFiles(): string[] {
  return ts.sys.readDirectory(
    srcRoot,
    ['.ts', '.vue'],
    ['**/__tests__/**', '**/core/i18n/locales/**', '**/*.spec.ts', '**/*.test.ts'],
    ['**/*'],
  )
}

function routeDisplayPath(path: string): string {
  return path.replace(/:[^/]+/g, 'sample')
}

describe('locale coverage', () => {
  it('keeps English and Chinese locale leaf keys in sync', () => {
    const enKeys = new Set(flattenLeafKeys(fullMessages.en))
    const zhKeys = new Set(flattenLeafKeys(fullMessages.zh))
    const findings = [
      ...[...zhKeys].filter((key) => !enKeys.has(key)).sort().map((key) => `missing in English: ${key}`),
      ...[...enKeys].filter((key) => !zhKeys.has(key)).sort().map((key) => `missing in Chinese: ${key}`),
    ]
    expect(findings.join('\n')).toBe('')
  })

  it('resolves every static locale key used by runtime source', () => {
    const findings: string[] = []
    for (const file of runtimeSourceFiles()) {
      const keys = sourceCandidateKeys(file)
      for (const locale of ['en', 'zh'] as const) {
        const missing = [...keys]
          .filter((key) => resolveMessage(fullMessages[locale], key) === undefined)
          .sort()
        if (missing.length > 0) {
          findings.push(`${relative(srcRoot, file)} [${locale}]: ${missing.join(', ')}`)
        }
      }
    }
    expect(findings.join('\n')).toBe('')
  }, 60_000)

  it('loads every statically reachable message for every route', () => {
    const findings: string[] = []
    const appFiles = dependencyClosure([appFile])
    const componentFilesCache = new Map<string, Set<string>>()
    for (const { path, component, metaKeys } of parseRouteSources()) {
      const componentFile = resolveSourceImport(routesFile, component)
      expect(componentFile, component).toBeDefined()
      let componentFiles = componentFilesCache.get(componentFile!)
      if (!componentFiles) {
        componentFiles = dependencyClosure([componentFile!])
        componentFilesCache.set(componentFile!, componentFiles)
      }
      const files = new Set([...appFiles, ...componentFiles])
      const keys = new Set(metaKeys)
      for (const file of files) {
        for (const key of sourceCandidateKeys(file, true)) keys.add(key)
      }

      const scopes = getLocaleScopesForRoute(routeDisplayPath(path))
      for (const locale of ['en', 'zh'] as const) {
        const messages = mergeMessages(scopes.map((scope) => localeScopes[locale][scope]))
        const missing = [...keys]
          .filter((key) => resolveMessage(messages, key) === undefined)
          .sort()
        if (missing.length > 0) {
          findings.push(
            `${path} [${locale}; ${scopes.join('+')}]: ${missing.join(', ')}`,
          )
        }
      }
    }
    expect(findings.join('\n')).toBe('')
  }, 60_000)
})
