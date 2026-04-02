import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getMock, postMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
}))

vi.mock('./client', () => ({
  apiClient: {
    get: getMock,
    post: postMock,
  },
}))

import { providerHealthApi } from './providerHealth'

describe('providerHealthApi', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
  })

  it('loads provider health from the aggregated management endpoint', async () => {
    getMock.mockResolvedValue({ providers: [] })

    await providerHealthApi.getProviders()

    expect(getMock).toHaveBeenCalledWith('/provider-health')
  })

  it('posts runtime reset payload to the reset endpoint', async () => {
    const payload = { locator: 'codex-api-key[0]', model: 'gpt-5.4' }
    postMock.mockResolvedValue({ matched_auths: [], cleared_models: [], providers: [] })

    await providerHealthApi.reset(payload)

    expect(postMock).toHaveBeenCalledWith('/provider-health/reset', payload)
  })
})
