import { describe, expect, it } from 'vitest'
import type { ProviderHealthProvider } from '@/services/api/providerHealth'
import type {
  GeminiKeyConfig,
  OpenAIProviderConfig,
  ProviderKeyConfig,
} from '@/types'
import { buildAiProvidersDisplayState } from './aiProvidersDisplayState'

describe('buildAiProvidersDisplayState', () => {
  it('overlays config-backed provider models from provider-health locator', () => {
    const codexConfigs: ProviderKeyConfig[] = [
      {
        apiKey: 'codex-key-1',
        models: [{ name: 'old-model' }],
      },
    ]

    const result = buildAiProvidersDisplayState({
      geminiKeys: [],
      codexConfigs,
      claudeConfigs: [],
      vertexConfigs: [],
      openaiProviders: [],
      healthProviders: [
        {
          key: 'codex-api-key[0]',
          kind: 'codex-api-key',
          config_locator: 'codex-api-key[0]',
          effective_models: ['gpt-5.4-mini', 'gpt-5.4'],
        },
      ],
    })

    expect(result.codexConfigs[0]?.models).toEqual([
      { name: 'gpt-5.4-mini' },
      { name: 'gpt-5.4' },
    ])
    expect(codexConfigs[0]?.models).toEqual([{ name: 'old-model' }])
  })

  it('matches openai-compatible providers by normalized name', () => {
    const openaiProviders: OpenAIProviderConfig[] = [
      {
        name: 'SaberRC',
        baseUrl: 'https://example.com/v1',
        apiKeyEntries: [{ apiKey: 'sk-test' }],
        models: [{ name: 'declared-only' }],
      },
    ]

    const healthProviders: ProviderHealthProvider[] = [
      {
        key: 'saberrc',
        kind: 'openai-compatibility',
        name: 'SaberRC',
        effective_models: ['claude-opus-4-1', 'gpt-5.4-mini'],
      },
    ]

    const result = buildAiProvidersDisplayState({
      geminiKeys: [],
      codexConfigs: [],
      claudeConfigs: [],
      vertexConfigs: [],
      openaiProviders,
      healthProviders,
    })

    expect(result.openaiProviders[0]?.models).toEqual([
      { name: 'claude-opus-4-1' },
      { name: 'gpt-5.4-mini' },
    ])
  })

  it('keeps config models when no provider-health match exists', () => {
    const geminiKeys: GeminiKeyConfig[] = [
      {
        apiKey: 'gemini-key-1',
        models: [{ name: 'gemini-2.5-pro' }],
      },
    ]

    const result = buildAiProvidersDisplayState({
      geminiKeys,
      codexConfigs: [],
      claudeConfigs: [],
      vertexConfigs: [],
      openaiProviders: [],
      healthProviders: [],
    })

    expect(result.geminiKeys[0]?.models).toEqual([{ name: 'gemini-2.5-pro' }])
  })

  it('treats matched provider with empty effective models as empty display models', () => {
    const claudeConfigs: ProviderKeyConfig[] = [
      {
        apiKey: 'claude-key-1',
        models: [{ name: 'claude-opus-4-1' }],
      },
    ]

    const result = buildAiProvidersDisplayState({
      geminiKeys: [],
      codexConfigs: [],
      claudeConfigs,
      vertexConfigs: [],
      openaiProviders: [],
      healthProviders: [
        {
          key: 'claude-api-key[0]',
          kind: 'claude-api-key',
          config_locator: 'claude-api-key[0]',
        },
      ],
    })

    expect(result.claudeConfigs[0]?.models).toEqual([])
  })
})
