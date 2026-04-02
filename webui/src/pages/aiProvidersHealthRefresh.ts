import { providerHealthApi } from '@/services/api/providerHealth'
import type { ProviderHealthProvider } from '@/services/api/providerHealth'

interface RefreshProviderHealthOptions {
  setHealthProviders: (providers: ProviderHealthProvider[]) => void
}

interface RunWithProviderHealthRefreshOptions extends RefreshProviderHealthOptions {
  mutation: () => Promise<unknown>
}

export async function refreshProviderHealth({
  setHealthProviders,
}: RefreshProviderHealthOptions): Promise<ProviderHealthProvider[]> {
  const response = await providerHealthApi.getProviders()
  const providers = response?.providers || []
  setHealthProviders(providers)
  return providers
}

export async function runWithProviderHealthRefresh({
  mutation,
  setHealthProviders,
}: RunWithProviderHealthRefreshOptions): Promise<void> {
  await mutation()
  await refreshProviderHealth({ setHealthProviders })
}
