import { describe, expect, it } from 'vitest'
import { buildProviderRowKey } from './keying'

describe('buildProviderRowKey', () => {
  it('keeps rows unique when apiKey is duplicated across different upstreams', () => {
    const first = buildProviderRowKey(
      {
        apiKey: 'sk',
        baseUrl: 'https://api.amethyst.ltd/v1',
      },
      4
    )

    const second = buildProviderRowKey(
      {
        apiKey: 'sk',
        baseUrl: 'https://cx-nya.zeabur.app/v1',
      },
      5
    )

    expect(first).not.toBe(second)
  })

  it('stays unique even when apiKey and baseUrl are both repeated', () => {
    const first = buildProviderRowKey(
      {
        apiKey: 'same-key',
        baseUrl: 'https://right.codes/codex/v1',
      },
      2
    )

    const second = buildProviderRowKey(
      {
        apiKey: 'same-key',
        baseUrl: 'https://right.codes/codex/v1',
      },
      13
    )

    expect(first).not.toBe(second)
  })
})
