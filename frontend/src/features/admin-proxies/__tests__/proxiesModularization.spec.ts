import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const readFeatureSource = (relativePath: string) =>
  readFileSync(resolve(currentDir, relativePath), 'utf8')

const runtimeSources = {
  page: readFeatureSource('../presentation/pages/ProxiesPage.vue'),
  table: readFeatureSource('../presentation/widgets/ProxyTable.vue'),
  createDialog: readFeatureSource('../presentation/widgets/CreateProxyDialog.vue'),
  editDialog: readFeatureSource('../presentation/widgets/EditProxyDialog.vue'),
  autoAssignmentDialog: readFeatureSource('../presentation/widgets/ProxyAutoAssignmentDialog.vue'),
  pageDialogs: readFeatureSource('../presentation/widgets/ProxyPageDialogs.vue'),
  context: readFeatureSource('../presentation/proxyPageContext.ts')
}

describe('proxies page modularization', () => {
  it('keeps every runtime module below the feature maintenance target', () => {
    for (const source of Object.values(runtimeSources)) {
      expect(source.split('\n').length).toBeLessThanOrEqual(1500)
    }
  })

  it('uses static feature widgets and the owner datasource', () => {
    expect(runtimeSources.page).toContain(
      "import { proxiesAPI } from '@/features/admin-proxies/data/datasources/adminProxiesDatasource'"
    )
    expect(runtimeSources.page).toContain('<ProxyTable :context="proxyTableContext" />')
    expect(runtimeSources.page).toContain('<CreateProxyDialog :context="createProxyDialogContext" />')
    expect(runtimeSources.page).toContain('<EditProxyDialog :context="editProxyDialogContext" />')
    expect(runtimeSources.page).toContain('<ProxyAutoAssignmentDialog :context="proxyAutoAssignmentDialogContext" />')
    expect(runtimeSources.page).toContain('<ProxyPageDialogs :context="proxyPageDialogsContext" />')

    for (const source of Object.values(runtimeSources)) {
      expect(source).not.toContain('import(')
      expect(source).not.toContain("from '@/api")
    }
  })

  it('preserves request cancellation, search debounce, and mount cleanup in the page owner', () => {
    expect(runtimeSources.page).toContain('const currentAbortController = new AbortController()')
    expect(runtimeSources.page).toContain('{ signal: currentAbortController.signal }')
    expect(runtimeSources.page).toContain('abortController?.abort()')
    expect(runtimeSources.page).toContain('searchTimeout = setTimeout(() => {')
    expect(runtimeSources.page).toContain('}, 300)')
    expect(runtimeSources.page).toContain('loadProxies()\n  loadBackupProxyOptions()')
    expect(runtimeSources.page).toContain("document.addEventListener('click', closeCopyMenu)")
    expect(runtimeSources.page).toContain("document.removeEventListener('click', closeCopyMenu)")
  })

  it('preserves editor form ids, submit owners, and import refresh wiring', () => {
    expect(runtimeSources.createDialog).toContain('id="create-proxy-form"')
    expect(runtimeSources.createDialog).toContain('@submit.prevent="handleCreateProxy"')
    expect(runtimeSources.createDialog).toContain('@click="handleBatchCreate"')
    expect(runtimeSources.editDialog).toContain('id="edit-proxy-form"')
    expect(runtimeSources.editDialog).toContain('@submit.prevent="handleUpdateProxy"')
    expect(runtimeSources.editDialog).toContain('@input="editPasswordDirty = true"')
    expect(runtimeSources.pageDialogs).toContain("import ImportDataModal from './ImportDataDialog.vue'")
    expect(runtimeSources.pageDialogs).toContain('@imported="handleDataImported"')
  })
})
