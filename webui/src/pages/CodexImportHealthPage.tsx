import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'
import { Card } from '@/components/ui/Card'
import { EmptyState } from '@/components/ui/EmptyState'
import { Input } from '@/components/ui/Input'
import { Select, type SelectOption } from '@/components/ui/Select'
import {
  providerHealthApi,
  type ProviderHealthModel,
  type ProviderHealthProvider,
} from '@/services/api'
import { useHeaderRefresh } from '@/hooks/useHeaderRefresh'
import { useNotificationStore } from '@/stores'
import {
  buildModelResetPayload,
  buildProviderResetPayload,
  buildProviderSummary,
  filterProviders,
  hasRuntimeBlockedModel,
  isDiscoveryFresh,
  isImportHealthy,
} from './providerHealthViewState'
import styles from './CodexImportHealthPage.module.scss'

type RuntimeFilterValue = 'all' | 'blocked' | 'healthy'
type DiscoveryFilterValue = 'all' | 'fresh' | 'problem'

const normalizeText = (value?: string) => (value || '').trim()

const formatTimestamp = (value?: string, language?: string) => {
  const normalized = normalizeText(value)
  if (!normalized) {
    return ''
  }

  const date = new Date(normalized)
  if (Number.isNaN(date.getTime())) {
    return normalized
  }

  return date.toLocaleString(language)
}

const getProviderRowKey = (provider: ProviderHealthProvider) =>
  normalizeText(provider.key) ||
  normalizeText(provider.config_locator) ||
  normalizeText(provider.name) ||
  'provider'

const getModelActionKey = (provider: ProviderHealthProvider, model: string) =>
  `${getProviderRowKey(provider)}::${normalizeText(model)}`

const hasRuntimeIssues = (provider: ProviderHealthProvider) =>
  Boolean(provider.runtime_unavailable || hasRuntimeBlockedModel(provider))

const getDiscoveryBadgeClass = (provider: ProviderHealthProvider) =>
  isDiscoveryFresh(provider) ? styles.statusHealthy : styles.statusWarning

const getRuntimeBadgeClass = (provider: ProviderHealthProvider) =>
  hasRuntimeIssues(provider) ? styles.statusProblem : styles.statusHealthy

const getImportBadgeClass = (provider: ProviderHealthProvider) => {
  if (!provider.import_health?.health?.status) {
    return styles.statusNeutral
  }

  return isImportHealthy(provider) ? styles.statusHealthy : styles.statusProblem
}

const getRuntimeModels = (provider: ProviderHealthProvider): ProviderHealthModel[] =>
  Array.isArray(provider.model_health) ? provider.model_health : []

export function CodexImportHealthPage() {
  const { t, i18n } = useTranslation()
  const { showNotification } = useNotificationStore()
  const [providers, setProviders] = useState<ProviderHealthProvider[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [search, setSearch] = useState('')
  const [runtimeFilter, setRuntimeFilter] = useState<RuntimeFilterValue>('all')
  const [discoveryFilter, setDiscoveryFilter] = useState<DiscoveryFilterValue>('all')
  const [kindFilter, setKindFilter] = useState('all')
  const [resettingProviderKey, setResettingProviderKey] = useState<string | null>(null)
  const [resettingModelKey, setResettingModelKey] = useState<string | null>(null)

  const loadProviders = useCallback(async () => {
    setLoading(true)
    setError('')

    try {
      const data = await providerHealthApi.getProviders()
      setProviders(Array.isArray(data.providers) ? data.providers : [])
    } catch (loadError: unknown) {
      const message =
        loadError instanceof Error ? loadError.message : typeof loadError === 'string' ? loadError : ''
      setError(message || t('common.unknown_error'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void loadProviders()
  }, [loadProviders])

  useHeaderRefresh(loadProviders)

  const summary = useMemo(() => buildProviderSummary(providers), [providers])

  const runtimeOptions = useMemo<SelectOption[]>(
    () => [
      { value: 'all', label: t('codex_import_health.runtime_filter_all') },
      { value: 'blocked', label: t('codex_import_health.runtime_filter_blocked') },
      { value: 'healthy', label: t('codex_import_health.runtime_filter_healthy') },
    ],
    [t]
  )

  const discoveryOptions = useMemo<SelectOption[]>(
    () => [
      { value: 'all', label: t('codex_import_health.discovery_filter_all') },
      { value: 'fresh', label: t('codex_import_health.discovery_filter_fresh') },
      { value: 'problem', label: t('codex_import_health.discovery_filter_problem') },
    ],
    [t]
  )

  const kindOptions = useMemo<SelectOption[]>(() => {
    const kinds = Array.from(
      new Set(
        providers
          .map((provider) => normalizeText(provider.kind))
          .filter(Boolean)
      )
    ).sort((left, right) => left.localeCompare(right))

    return [
      { value: 'all', label: t('codex_import_health.kind_filter_all') },
      ...kinds.map((kind) => ({ value: kind, label: kind })),
    ]
  }, [providers, t])

  const filteredProviders = useMemo(
    () =>
      filterProviders(providers, {
        search,
        runtimeFilter,
        discoveryFilter,
        kindFilter,
      }),
    [providers, search, runtimeFilter, discoveryFilter, kindFilter]
  )

  const handleResetProvider = useCallback(
    async (provider: ProviderHealthProvider) => {
      const payload = buildProviderResetPayload(provider)
      if (!payload.key && !payload.locator) {
        showNotification(t('provider_health.reset_missing_target'), 'error')
        return
      }

      const actionKey = getProviderRowKey(provider)
      setResettingProviderKey(actionKey)

      try {
        const result = await providerHealthApi.reset(payload)
        setProviders(Array.isArray(result.providers) ? result.providers : [])
        showNotification(t('provider_health.reset_provider_success'), 'success')
      } catch (resetError: unknown) {
        const message =
          resetError instanceof Error
            ? resetError.message
            : typeof resetError === 'string'
              ? resetError
              : ''
        showNotification(
          `${t('provider_health.reset_provider_failed')}${message ? `: ${message}` : ''}`,
          'error'
        )
      } finally {
        setResettingProviderKey(null)
      }
    },
    [showNotification, t]
  )

  const handleResetModel = useCallback(
    async (provider: ProviderHealthProvider, model: string) => {
      const payload = buildModelResetPayload(provider, model)
      if ((!payload.key && !payload.locator) || !payload.model) {
        showNotification(t('provider_health.reset_missing_target'), 'error')
        return
      }

      const actionKey = getModelActionKey(provider, model)
      setResettingModelKey(actionKey)

      try {
        const result = await providerHealthApi.reset(payload)
        setProviders(Array.isArray(result.providers) ? result.providers : [])
        showNotification(t('provider_health.reset_model_success'), 'success')
      } catch (resetError: unknown) {
        const message =
          resetError instanceof Error
            ? resetError.message
            : typeof resetError === 'string'
              ? resetError
              : ''
        showNotification(
          `${t('provider_health.reset_model_failed')}${message ? `: ${message}` : ''}`,
          'error'
        )
      } finally {
        setResettingModelKey(null)
      }
    },
    [showNotification, t]
  )

  return (
    <div className={styles.container}>
      <div className={styles.pageHeader}>
        <div>
          <h1 className={styles.pageTitle}>{t('codex_import_health.title')}</h1>
          <p className={styles.description}>{t('codex_import_health.description')}</p>
        </div>
        <Button variant="secondary" size="sm" onClick={() => void loadProviders()} disabled={loading}>
          {loading ? t('common.loading') : t('common.refresh')}
        </Button>
      </div>

      <Card>
        <div className={styles.notice}>
          <span className={styles.noticeBadge}>{t('common.info')}</span>
          <span>{t('codex_import_health.read_only_notice')}</span>
        </div>
      </Card>

      {error && <div className={styles.errorBox}>{error}</div>}

      <div className={styles.summaryGrid}>
        <Card className={styles.summaryCard}>
          <div className={styles.summaryLabel}>{t('codex_import_health.summary_total')}</div>
          <div className={styles.summaryValue}>{summary.total}</div>
        </Card>
        <Card className={styles.summaryCard}>
          <div className={styles.summaryLabel}>{t('codex_import_health.summary_discovery_problem')}</div>
          <div className={`${styles.summaryValue} ${styles.warningText}`}>{summary.discoveryProblem}</div>
        </Card>
        <Card className={styles.summaryCard}>
          <div className={styles.summaryLabel}>{t('codex_import_health.summary_runtime_blocked')}</div>
          <div className={`${styles.summaryValue} ${styles.problemText}`}>{summary.runtimeBlocked}</div>
        </Card>
        <Card className={styles.summaryCard}>
          <div className={styles.summaryLabel}>{t('codex_import_health.summary_import_problem')}</div>
          <div className={`${styles.summaryValue} ${styles.problemText}`}>{summary.importProblem}</div>
        </Card>
      </div>

      <Card
        title={t('codex_import_health.table_title')}
        extra={
          <div className={styles.filters}>
            <div className={styles.filterItem}>
              <label>{t('codex_import_health.runtime_filter_label')}</label>
              <Select
                value={runtimeFilter}
                options={runtimeOptions}
                onChange={(value) => setRuntimeFilter(value as RuntimeFilterValue)}
                ariaLabel={t('codex_import_health.runtime_filter_label')}
                className={styles.statusSelect}
                fullWidth={false}
              />
            </div>
            <div className={styles.filterItem}>
              <label>{t('codex_import_health.discovery_filter_label')}</label>
              <Select
                value={discoveryFilter}
                options={discoveryOptions}
                onChange={(value) => setDiscoveryFilter(value as DiscoveryFilterValue)}
                ariaLabel={t('codex_import_health.discovery_filter_label')}
                className={styles.statusSelect}
                fullWidth={false}
              />
            </div>
            <div className={styles.filterItem}>
              <label>{t('codex_import_health.kind_filter_label')}</label>
              <Select
                value={kindFilter}
                options={kindOptions}
                onChange={setKindFilter}
                ariaLabel={t('codex_import_health.kind_filter_label')}
                className={styles.statusSelect}
                fullWidth={false}
              />
            </div>
            <div className={styles.filterItem}>
              <Input
                label={t('codex_import_health.search_label')}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t('codex_import_health.search_placeholder')}
              />
            </div>
          </div>
        }
      >
        {filteredProviders.length === 0 ? (
          <EmptyState
            title={
              loading ? t('common.loading') : t('codex_import_health.empty_title')
            }
            description={
              loading
                ? t('codex_import_health.loading_desc')
                : t('codex_import_health.empty_desc')
            }
            action={
              !loading ? (
                <Button variant="secondary" size="sm" onClick={() => void loadProviders()}>
                  {t('common.refresh')}
                </Button>
              ) : undefined
            }
          />
        ) : (
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>{t('codex_import_health.table_provider')}</th>
                  <th>{t('codex_import_health.table_discovery')}</th>
                  <th>{t('codex_import_health.table_runtime')}</th>
                  <th>{t('codex_import_health.table_import')}</th>
                  <th>{t('codex_import_health.table_effective_models')}</th>
                  <th>{t('codex_import_health.table_actions')}</th>
                </tr>
              </thead>
              <tbody>
                {filteredProviders.map((provider) => {
                  const rowKey = getProviderRowKey(provider)
                  const runtimeModels = getRuntimeModels(provider)
                  const hasBlockedRuntime = hasRuntimeIssues(provider)
                  const importStatus = normalizeText(provider.import_health?.health?.status) || t('common.not_set')
                  const importCheckedAt =
                    formatTimestamp(provider.import_health?.health?.checked_at, i18n.language) ||
                    t('common.not_set')
                  const effectiveModels =
                    Array.isArray(provider.effective_models) && provider.effective_models.length > 0
                      ? provider.effective_models
                      : []

                  return (
                    <tr key={rowKey}>
                      <td>
                        <div className={styles.providerCell}>
                          <div className={styles.providerName}>{provider.name || provider.key}</div>
                          <div className={styles.providerSub}>{provider.key}</div>
                          <div className={styles.inlineMeta}>
                            {provider.kind && <span className={styles.subtleBadge}>{provider.kind}</span>}
                            {provider.config_locator && (
                              <span className={styles.providerSub}>{provider.config_locator}</span>
                            )}
                          </div>
                          {Array.isArray(provider.auth_ids) && provider.auth_ids.length > 0 && (
                            <div className={styles.providerSub}>
                              {t('codex_import_health.auth_ids')}: {provider.auth_ids.join(', ')}
                            </div>
                          )}
                        </div>
                      </td>
                      <td>
                        <div className={styles.discoveryCell}>
                          <span className={`${styles.statusBadge} ${getDiscoveryBadgeClass(provider)}`}>
                            {normalizeText(provider.discovery_state) || t('common.not_set')}
                          </span>
                        </div>
                      </td>
                      <td>
                        <div className={styles.runtimeCell}>
                          <span className={`${styles.statusBadge} ${getRuntimeBadgeClass(provider)}`}>
                            {hasBlockedRuntime
                              ? t('codex_import_health.runtime_blocked')
                              : t('codex_import_health.runtime_healthy')}
                          </span>
                          {runtimeModels.length === 0 ? (
                            <span className={styles.emptyText}>
                              {t('codex_import_health.no_blocked_models')}
                            </span>
                          ) : (
                            <div className={styles.runtimeModelList}>
                              {runtimeModels.map((item) => {
                                const modelKey = getModelActionKey(provider, item.model)
                                return (
                                  <div key={modelKey} className={styles.runtimeModelItem}>
                                    <div className={styles.runtimeModelInfo}>
                                      <div className={styles.tagList}>
                                        <span className={styles.tag}>{item.model}</span>
                                        {item.status && (
                                          <span className={`${styles.statusBadge} ${styles.statusProblem}`}>
                                            {item.status}
                                          </span>
                                        )}
                                      </div>
                                      {item.status_message && (
                                        <div className={styles.providerSub}>{item.status_message}</div>
                                      )}
                                      {item.next_retry_after && (
                                        <div className={styles.providerSub}>
                                          {t('codex_import_health.next_retry_after')}:{' '}
                                          {formatTimestamp(item.next_retry_after, i18n.language)}
                                        </div>
                                      )}
                                    </div>
                                    <Button
                                      variant="secondary"
                                      size="sm"
                                      loading={resettingModelKey === modelKey}
                                      onClick={() => void handleResetModel(provider, item.model)}
                                    >
                                      {t('provider_health.reset_model')}
                                    </Button>
                                  </div>
                                )
                              })}
                            </div>
                          )}
                        </div>
                      </td>
                      <td>
                        <div className={styles.importCell}>
                          <span className={`${styles.statusBadge} ${getImportBadgeClass(provider)}`}>
                            {importStatus}
                          </span>
                          {provider.import_health?.health?.detail && (
                            <div className={styles.providerSub}>
                              {provider.import_health.health.detail}
                            </div>
                          )}
                          <div className={styles.providerSub}>
                            {t('codex_import_health.table_checked_at')}: {importCheckedAt}
                          </div>
                          {provider.import_health?.health?.base_url && (
                            <div className={styles.providerSub}>
                              {provider.import_health.health.base_url}
                            </div>
                          )}
                        </div>
                      </td>
                      <td>
                        {effectiveModels.length === 0 ? (
                          <span className={styles.emptyText}>{t('common.not_set')}</span>
                        ) : (
                          <div className={styles.tagList}>
                            {effectiveModels.map((model) => (
                              <span key={`${rowKey}-${model}`} className={styles.tag}>
                                {model}
                              </span>
                            ))}
                          </div>
                        )}
                      </td>
                      <td>
                        <div className={styles.actionCell}>
                          {hasBlockedRuntime ? (
                            <Button
                              variant="secondary"
                              size="sm"
                              loading={resettingProviderKey === rowKey}
                              onClick={() => void handleResetProvider(provider)}
                            >
                              {t('provider_health.reset_provider')}
                            </Button>
                          ) : (
                            <span className={styles.emptyText}>
                              {t('codex_import_health.no_runtime_action')}
                            </span>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  )
}
