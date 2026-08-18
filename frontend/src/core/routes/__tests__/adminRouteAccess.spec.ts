import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import ts from 'typescript'
import { describe, expect, it } from 'vitest'

const routesFile = resolve(process.cwd(), 'src/core/routes/index.ts')

function property(object: ts.ObjectLiteralExpression, name: string): ts.PropertyAssignment | undefined {
  return object.properties.find((entry): entry is ts.PropertyAssignment => (
    ts.isPropertyAssignment(entry)
    && ((ts.isIdentifier(entry.name) || ts.isStringLiteral(entry.name)) && entry.name.text === name)
  ))
}

function adminRouteFindings(): string[] {
  const source = readFileSync(routesFile, 'utf8')
  const ast = ts.createSourceFile(routesFile, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  const findings: string[] = []

  ast.forEachChild(function visit(node) {
    if (
      ts.isVariableDeclaration(node)
      && ts.isIdentifier(node.name)
      && node.name.text === 'routes'
      && node.initializer
      && ts.isArrayLiteralExpression(node.initializer)
    ) {
      for (const element of node.initializer.elements) {
        if (!ts.isObjectLiteralExpression(element)) continue
        const pathEntry = property(element, 'path')
        if (!pathEntry || !ts.isStringLiteral(pathEntry.initializer)) continue
        const path = pathEntry.initializer.text
        if (!path.startsWith('/admin')) continue

        const metaEntry = property(element, 'meta')
        const meta = metaEntry?.initializer
        const requiresAdmin = meta && ts.isObjectLiteralExpression(meta)
          ? property(meta, 'requiresAdmin')?.initializer
          : undefined
        if (!requiresAdmin || requiresAdmin.kind !== ts.SyntaxKind.TrueKeyword) {
          findings.push(`${path}: missing meta.requiresAdmin = true`)
        }
      }
    }
    ts.forEachChild(node, visit)
  })

  return findings
}

describe('admin route access contract', () => {
  it('marks every /admin route as administrator-only UI', () => {
    expect(adminRouteFindings()).toEqual([])
  })
})
