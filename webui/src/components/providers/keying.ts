type ProviderRowKeyInput = {
  apiKey?: string
  baseUrl?: string
  prefix?: string
  name?: string
}

const normalizePart = (value?: string) => String(value ?? '').trim()

export const buildProviderRowKey = (item: ProviderRowKeyInput, index: number) => {
  const apiKey = normalizePart(item.apiKey)
  const baseUrl = normalizePart(item.baseUrl)
  const prefix = normalizePart(item.prefix)
  const name = normalizePart(item.name)

  return [apiKey || name || 'provider', baseUrl || 'no-base-url', prefix || 'no-prefix', index].join(
    '|'
  )
}
