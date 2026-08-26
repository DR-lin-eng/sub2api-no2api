/**
 * Return the canonical browser path used by the SPA router.
 *
 * Route records are authored with lowercase static segments. Vue Router's
 * default matcher accepts case variants, so normalize the pathname before
 * loading route-scoped resources or updating the browser URL. If a complete
 * path is supplied, its query/hash suffix remains byte-for-byte unchanged.
 */
export function canonicalizeRoutePath(pathname: string): string {
  const suffixStart = pathname.search(/[?#]/)
  const pathEnd = suffixStart === -1 ? pathname.length : suffixStart
  const path = pathname.slice(0, pathEnd)
  const normalizedPath = path.replace(/[A-Z]/g, (character) => character.toLowerCase())
  return `${normalizedPath}${pathname.slice(pathEnd)}`
}
