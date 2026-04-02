export interface ProviderHealthModelLike {
  model?: string
  runtime_unavailable?: boolean
}

export interface ProviderHealthImportHealthLike {
  health?: {
    status?: string
  }
}

export interface ProviderHealthProviderLike {
  key?: string
  kind?: string
  name?: string
  config_locator?: string
  discovery_state?: string
  runtime_unavailable?: boolean
  effective_models?: string[]
  model_health?: ProviderHealthModelLike[]
  import_health?: ProviderHealthImportHealthLike
}

export interface ProviderHealthSummary {
  total: number
  discoveryProblem: number
  runtimeBlocked: number
  importProblem: number
}

export interface ProviderHealthFilters {
  search: string
  runtimeFilter: 'all' | 'blocked' | 'healthy'
  discoveryFilter: 'all' | 'fresh' | 'problem'
  kindFilter: string
}

export interface ProviderHealthResetPayload {
  key?: string
  locator?: string
  model?: string
}

const normalizeText = (value?: string) => (value || '').trim()

export const isDiscoveryFresh = (provider: ProviderHealthProviderLike) =>
  normalizeText(provider.discovery_state).toLowerCase() === 'fresh'

export const hasRuntimeBlockedModel = (provider: ProviderHealthProviderLike) =>
  (provider.model_health || []).some((item) => item?.runtime_unavailable)

export const isImportHealthy = (provider: ProviderHealthProviderLike) =>
  normalizeText(provider.import_health?.health?.status).toLowerCase() === 'healthy'

export const hasImportHealth = (provider: ProviderHealthProviderLike) =>
  Boolean(normalizeText(provider.import_health?.health?.status))

const buildSearchText = (provider: ProviderHealthProviderLike) =>
  [
    provider.key,
    provider.kind,
    provider.name,
    provider.config_locator,
    provider.discovery_state,
    provider.effective_models?.join(' '),
    provider.model_health?.map((item) => item.model).join(' '),
    provider.import_health?.health?.status,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()

export const buildProviderSummary = (
  providers: ProviderHealthProviderLike[]
): ProviderHealthSummary => ({
  total: providers.length,
  discoveryProblem: providers.filter((provider) => !isDiscoveryFresh(provider)).length,
  runtimeBlocked: providers.filter(
    (provider) => provider.runtime_unavailable || hasRuntimeBlockedModel(provider)
  ).length,
  importProblem: providers.filter(
    (provider) => hasImportHealth(provider) && !isImportHealthy(provider)
  ).length,
})

export const filterProviders = <T extends ProviderHealthProviderLike>(
  providers: T[],
  filters: ProviderHealthFilters
) => {
  const keyword = normalizeText(filters.search).toLowerCase()
  const kindFilter = normalizeText(filters.kindFilter).toLowerCase()

  return providers.filter((provider) => {
    const matchesRuntime =
      filters.runtimeFilter === 'all' ||
      (filters.runtimeFilter === 'blocked'
        ? provider.runtime_unavailable || hasRuntimeBlockedModel(provider)
        : !(provider.runtime_unavailable || hasRuntimeBlockedModel(provider)))

    const matchesDiscovery =
      filters.discoveryFilter === 'all' ||
      (filters.discoveryFilter === 'fresh'
        ? isDiscoveryFresh(provider)
        : !isDiscoveryFresh(provider))

    const matchesKind = kindFilter === 'all' || normalizeText(provider.kind).toLowerCase() === kindFilter
    const matchesSearch = !keyword || buildSearchText(provider).includes(keyword)

    return matchesRuntime && matchesDiscovery && matchesKind && matchesSearch
  })
}

export const buildProviderResetPayload = (
  provider: Pick<ProviderHealthProviderLike, 'key' | 'config_locator'>
): ProviderHealthResetPayload => {
  const locator = normalizeText(provider.config_locator)
  if (locator) {
    return { locator }
  }

  const key = normalizeText(provider.key)
  return key ? { key } : {}
}

export const buildModelResetPayload = (
  provider: Pick<ProviderHealthProviderLike, 'key' | 'config_locator'>,
  model: string
): ProviderHealthResetPayload => {
  const payload = buildProviderResetPayload(provider)
  const normalizedModel = normalizeText(model)

  return normalizedModel ? { ...payload, model: normalizedModel } : payload
}
