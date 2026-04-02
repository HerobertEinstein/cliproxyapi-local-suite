import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getProvidersMock } = vi.hoisted(() => ({
  getProvidersMock: vi.fn(),
}))

vi.mock('@/services/api/providerHealth', () => ({
  providerHealthApi: {
    getProviders: getProvidersMock,
  },
}))

import {
  refreshProviderHealth,
  runWithProviderHealthRefresh,
} from './aiProvidersHealthRefresh'

describe('aiProvidersHealthRefresh', () => {
  beforeEach(() => {
    getProvidersMock.mockReset()
  })

  it('refreshes provider health after a successful provider toggle mutation', async () => {
    const setHealthProviders = vi.fn()
    const mutation = vi.fn().mockResolvedValue(undefined)
    const providers = [{ key: 'codex-api-key[0]', effective_models: ['gpt-5.4-mini'] }]
    getProvidersMock.mockResolvedValue({ providers })

    await runWithProviderHealthRefresh({ mutation, setHealthProviders })

    expect(mutation).toHaveBeenCalledTimes(1)
    expect(getProvidersMock).toHaveBeenCalledTimes(1)
    expect(setHealthProviders).toHaveBeenCalledWith(providers)
  })

  it('refreshes provider health after a successful provider deletion', async () => {
    const setHealthProviders = vi.fn()
    const mutation = vi.fn().mockResolvedValue({ status: 'ok' })
    const providers = [{ key: 'rightcode', effective_models: ['gpt-5.4-mini'] }]
    getProvidersMock.mockResolvedValue({ providers })

    await runWithProviderHealthRefresh({ mutation, setHealthProviders })

    expect(mutation).toHaveBeenCalledTimes(1)
    expect(getProvidersMock).toHaveBeenCalledTimes(1)
    expect(setHealthProviders).toHaveBeenCalledWith(providers)
  })

  it('does not refresh provider health when the mutation fails', async () => {
    const setHealthProviders = vi.fn()
    const mutationError = new Error('save failed')
    const mutation = vi.fn().mockRejectedValue(mutationError)

    await expect(
      runWithProviderHealthRefresh({ mutation, setHealthProviders })
    ).rejects.toThrow('save failed')

    expect(getProvidersMock).not.toHaveBeenCalled()
    expect(setHealthProviders).not.toHaveBeenCalled()
  })

  it('loads providers and updates local health state', async () => {
    const setHealthProviders = vi.fn()
    const providers = [{ key: 'claude-api-key[0]', effective_models: ['claude-opus-4-1'] }]
    getProvidersMock.mockResolvedValue({ providers })

    const result = await refreshProviderHealth({ setHealthProviders })

    expect(getProvidersMock).toHaveBeenCalledTimes(1)
    expect(setHealthProviders).toHaveBeenCalledWith(providers)
    expect(result).toEqual(providers)
  })
})
