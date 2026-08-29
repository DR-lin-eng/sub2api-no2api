import type {
  CustomModelConfig,
  ModelCapability,
} from '@/features/custom-model-config/domain/entities/customModelConfig'

interface CompiledModelCapabilities {
  exact: Map<string, ReadonlySet<ModelCapability>>
  prefixes: Array<{
    modelName: string
    capabilities: ReadonlySet<ModelCapability>
  }>
}

let compiledCapabilities: CompiledModelCapabilities | null = null

export function initializeModelCapabilities(configs: CustomModelConfig[]): void {
  const exact = new Map<string, ReadonlySet<ModelCapability>>()
  const prefixes: CompiledModelCapabilities['prefixes'] = []
  for (const config of configs) {
    const modelName = config.model_name.trim().toLowerCase()
    if (!modelName) continue
    const capabilities = new Set(config.capabilities)
    if (config.prefix_match) {
      prefixes.push({ modelName, capabilities })
    } else {
      exact.set(modelName, capabilities)
    }
  }
  prefixes.sort((left, right) => right.modelName.length - left.modelName.length)
  compiledCapabilities = { exact, prefixes }
}

export function clearModelCapabilities(): void {
  compiledCapabilities = null
}

function resolveConfiguredCapabilities(model: string): ReadonlySet<ModelCapability> | null {
  if (!compiledCapabilities) return null
  const normalized = model.trim().toLowerCase()
  const exact = compiledCapabilities.exact.get(normalized)
  if (exact) return exact
  for (const entry of compiledCapabilities.prefixes) {
    if (normalized.startsWith(entry.modelName)) return entry.capabilities
  }
  return null
}

export function isMediaStudioImageModel(model: string): boolean {
  const normalized = model.trim().toLowerCase()
  const configuredCapabilities = resolveConfiguredCapabilities(normalized)
  if (configuredCapabilities) return configuredCapabilities.has('image')
  return normalized.startsWith('gpt-image-') || (
    normalized.startsWith('grok-imagine') && !normalized.startsWith('grok-imagine-video')
  )
}

export function isMediaStudioVideoModel(model: string): boolean {
  const normalized = model.trim().toLowerCase()
  const configuredCapabilities = resolveConfiguredCapabilities(normalized)
  if (configuredCapabilities) return configuredCapabilities.has('video')
  return normalized.startsWith('grok-imagine-video')
}

export function isMediaStudioAudioModel(model: string): boolean {
  const configuredCapabilities = resolveConfiguredCapabilities(model)
  return configuredCapabilities?.has('audio') ?? false
}
