import { apiClient } from './client'

export interface ProviderHealthModel {
  model: string
  status?: string
  status_message?: string
  runtime_unavailable: boolean
  next_retry_after?: string
}

export interface ProviderHealthImportStatus {
  provider?: string
  base_url?: string
  status?: string
  detail?: string
  checked_at?: string
}

export interface ProviderHealthImportHealth {
  config_locator?: string
  provider?: string
  endpoint_url?: string
  config_base_url?: string
  effective_base_url?: string
  default_model?: string
  template_models?: string[]
  discovered_models?: string[]
  effective_models?: string[]
  effective_source?: string
  health?: ProviderHealthImportStatus
}

export interface ProviderHealthProvider {
  key: string
  kind?: string
  name?: string
  config_locator?: string
  auth_ids?: string[]
  discovery_state?: string
  effective_models?: string[]
  runtime_unavailable?: boolean
  model_health?: ProviderHealthModel[]
  import_health?: ProviderHealthImportHealth
}

export interface ProviderHealthResponse {
  providers: ProviderHealthProvider[]
}

export interface ProviderHealthResetRequest {
  key?: string
  locator?: string
  auth_id?: string
  model?: string
}

export interface ProviderHealthResetResponse {
  matched_auths: string[]
  cleared_models: string[]
  providers: ProviderHealthProvider[]
}

export const providerHealthApi = {
  getProviders: () => apiClient.get<ProviderHealthResponse>('/provider-health'),
  reset: (payload: ProviderHealthResetRequest) =>
    apiClient.post<ProviderHealthResetResponse>('/provider-health/reset', payload),
}
