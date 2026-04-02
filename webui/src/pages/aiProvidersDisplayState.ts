import type { ProviderHealthProvider } from '@/services/api/providerHealth'
import type {
  GeminiKeyConfig,
  ModelAlias,
  OpenAIProviderConfig,
  ProviderKeyConfig,
} from '@/types'

interface AiProvidersDisplayStateInput {
  geminiKeys: GeminiKeyConfig[]
  codexConfigs: ProviderKeyConfig[]
  claudeConfigs: ProviderKeyConfig[]
  vertexConfigs: ProviderKeyConfig[]
  openaiProviders: OpenAIProviderConfig[]
  healthProviders: ProviderHealthProvider[]
}

interface AiProvidersDisplayState {
  geminiKeys: GeminiKeyConfig[]
  codexConfigs: ProviderKeyConfig[]
  claudeConfigs: ProviderKeyConfig[]
  vertexConfigs: ProviderKeyConfig[]
  openaiProviders: OpenAIProviderConfig[]
}

const normalizeText = (value?: string) => (value || '').trim()

const normalizeLookupKey = (value?: string) => normalizeText(value).toLowerCase()

const normalizeOpenAICompatProviderKey = (name?: string) => {
  const normalized = normalizeLookupKey(name)
  return normalized || 'openai-compatibility'
}

const toModelAliases = (models?: string[]): ModelAlias[] =>
  (models || [])
    .map((model) => normalizeText(model))
    .filter(Boolean)
    .map((name) => ({ name }))

const buildHealthIndexes = (providers: ProviderHealthProvider[]) => {
  const byLocator = new Map<string, ProviderHealthProvider>()
  const byOpenAIKey = new Map<string, ProviderHealthProvider>()

  providers.forEach((provider) => {
    const locator = normalizeLookupKey(provider.config_locator || provider.key)
    if (locator && !byLocator.has(locator)) {
      byLocator.set(locator, provider)
    }

    const isOpenAICompat = normalizeLookupKey(provider.kind) === 'openai-compatibility'
    if (!isOpenAICompat) {
      return
    }

    const candidateKeys = [
      normalizeOpenAICompatProviderKey(provider.key),
      normalizeOpenAICompatProviderKey(provider.name),
    ]
    candidateKeys.forEach((key) => {
      if (key && !byOpenAIKey.has(key)) {
        byOpenAIKey.set(key, provider)
      }
    })
  })

  return { byLocator, byOpenAIKey }
}

const overlayModels = <T extends { models?: ModelAlias[] }>(
  configs: T[],
  resolveProvider: (config: T, index: number) => ProviderHealthProvider | undefined
) =>
  configs.map((config, index) => {
    const provider = resolveProvider(config, index)
    if (!provider) {
      return config
    }
    return {
      ...config,
      models: toModelAliases(provider.effective_models),
    }
  })

export const buildAiProvidersDisplayState = ({
  geminiKeys,
  codexConfigs,
  claudeConfigs,
  vertexConfigs,
  openaiProviders,
  healthProviders,
}: AiProvidersDisplayStateInput): AiProvidersDisplayState => {
  const { byLocator, byOpenAIKey } = buildHealthIndexes(healthProviders)
  const resolveConfigProvider = (kind: string, index: number) =>
    byLocator.get(normalizeLookupKey(`${kind}[${index}]`))

  return {
    geminiKeys: overlayModels(geminiKeys, (_, index) =>
      resolveConfigProvider('gemini-api-key', index)
    ),
    codexConfigs: overlayModels(codexConfigs, (_, index) =>
      resolveConfigProvider('codex-api-key', index)
    ),
    claudeConfigs: overlayModels(claudeConfigs, (_, index) =>
      resolveConfigProvider('claude-api-key', index)
    ),
    vertexConfigs: overlayModels(vertexConfigs, (_, index) =>
      resolveConfigProvider('vertex-api-key', index)
    ),
    openaiProviders: overlayModels(openaiProviders, (config) =>
      byOpenAIKey.get(normalizeOpenAICompatProviderKey(config.name))
    ),
  }
}
