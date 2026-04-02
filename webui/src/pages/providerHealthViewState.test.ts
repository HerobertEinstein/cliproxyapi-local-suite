import { describe, expect, it } from 'vitest'
import {
  buildModelResetPayload,
  buildProviderResetPayload,
  buildProviderSummary,
  filterProviders,
} from './providerHealthViewState'

const providers = [
  {
    key: 'codex-api-key[0]',
    kind: 'codex-api-key',
    config_locator: 'codex-api-key[0]',
    discovery_state: 'fresh',
    runtime_unavailable: true,
    effective_models: ['gpt-5.4'],
    model_health: [
      {
        model: 'gpt-5.4',
        runtime_unavailable: true,
        status: 'error',
      },
    ],
    import_health: {
      health: {
        status: 'healthy',
      },
    },
  },
  {
    key: 'claude-api-key[0]',
    kind: 'claude-api-key',
    config_locator: 'claude-api-key[0]',
    discovery_state: 'stale',
    runtime_unavailable: false,
    effective_models: ['claude-opus-4-1'],
    model_health: [],
    import_health: {
      health: {
        status: 'error',
      },
    },
  },
]

describe('buildProviderSummary', () => {
  it('counts discovery, runtime, and import health independently', () => {
    expect(buildProviderSummary(providers)).toEqual({
      total: 2,
      discoveryProblem: 1,
      runtimeBlocked: 1,
      importProblem: 1,
    })
  })

  it('does not count missing import health as an import problem', () => {
    expect(
      buildProviderSummary([
        {
          key: 'vertex-api-key[0]',
          discovery_state: 'fresh',
          runtime_unavailable: false,
          model_health: [],
        },
      ])
    ).toEqual({
      total: 1,
      discoveryProblem: 0,
      runtimeBlocked: 0,
      importProblem: 0,
    })
  })
})

describe('filterProviders', () => {
  it('filters by runtime state and keyword', () => {
    const result = filterProviders(providers, {
      search: 'codex',
      runtimeFilter: 'blocked',
      discoveryFilter: 'all',
      kindFilter: 'all',
    })

    expect(result).toHaveLength(1)
    expect(result[0]?.key).toBe('codex-api-key[0]')
  })
})

describe('reset payload builders', () => {
  it('prefers locator for provider reset', () => {
    expect(buildProviderResetPayload(providers[0])).toEqual({
      locator: 'codex-api-key[0]',
    })
  })

  it('falls back to key when locator is absent', () => {
    expect(
      buildProviderResetPayload({
        key: 'openai-compatibility[test]',
      })
    ).toEqual({
      key: 'openai-compatibility[test]',
    })
  })

  it('includes model for model reset', () => {
    expect(buildModelResetPayload(providers[0], 'gpt-5.4')).toEqual({
      locator: 'codex-api-key[0]',
      model: 'gpt-5.4',
    })
  })
})
