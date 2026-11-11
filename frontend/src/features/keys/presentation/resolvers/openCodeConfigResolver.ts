import {
  antigravityGeminiModelCatalog,
  claudeModelCatalog,
  claude52ModelCatalog,
  geminiModelCatalog,
  grokModelCatalog,
  openAIModelCatalog,
  type OpenCodeModelCatalog
} from '@/features/keys/presentation/resolvers/openCodeModelCatalogs'

export type OpenCodeProviderId =
  | 'anthropic'
  | 'openai'
  | 'gemini'
  | 'antigravity-claude'
  | 'antigravity-gemini'
  | 'grok'

interface OpenCodeProviderConfig {
  options: {
    baseURL: string
    apiKey: string
  }
  npm?: '@ai-sdk/anthropic' | '@ai-sdk/google' | '@ai-sdk/openai'
  name?: string
  models?: OpenCodeModelCatalog
}

export interface OpenCodeConfigInput {
  platform: OpenCodeProviderId
  baseUrl: string
  apiKey: string
  hint: string
  pathLabel?: string
}

export interface OpenCodeFileConfig {
  path: string
  content: string
  hint: string
}

function resolveProviderConfig(
  platform: OpenCodeProviderId,
  baseUrl: string,
  apiKey: string
): OpenCodeProviderConfig {
  const provider: OpenCodeProviderConfig = {
    options: {
      baseURL: baseUrl,
      apiKey
    }
  }

  switch (platform) {
    case 'gemini':
      provider.npm = '@ai-sdk/google'
      provider.models = geminiModelCatalog
      break
    case 'anthropic':
      provider.npm = '@ai-sdk/anthropic'
      provider.models = claude52ModelCatalog
      break
    case 'antigravity-claude':
      provider.npm = '@ai-sdk/anthropic'
      provider.name = 'Antigravity (Claude)'
      provider.models = claudeModelCatalog
      break
    case 'antigravity-gemini':
      provider.npm = '@ai-sdk/google'
      provider.name = 'Antigravity (Gemini)'
      provider.models = antigravityGeminiModelCatalog
      break
    case 'openai':
      provider.models = openAIModelCatalog
      break
    case 'grok':
      provider.npm = '@ai-sdk/openai'
      provider.name = 'Grok'
      provider.models = grokModelCatalog
      break
  }

  return provider
}

export function generateOpenCodeConfig({
  platform,
  baseUrl,
  apiKey,
  hint,
  pathLabel
}: OpenCodeConfigInput): OpenCodeFileConfig {
  const provider = {
    [platform]: resolveProviderConfig(platform, baseUrl, apiKey)
  }
  const agent = platform === 'openai'
    ? {
        build: {
          options: {
            store: false
          }
        },
        plan: {
          options: {
            store: false
          }
        }
      }
    : undefined

  const content = JSON.stringify(
    {
      provider,
      ...(agent ? { agent } : {}),
      $schema: 'https://opencode.ai/config.json'
    },
    null,
    2
  )

  return {
    path: pathLabel ?? 'opencode.json',
    content,
    hint
  }
}
