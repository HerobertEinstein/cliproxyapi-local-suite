import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { Select, type SelectOption } from '@/components/ui/Select';
import { configApi } from '@/services/api';
import { useConfigStore, useNotificationStore } from '@/stores';
import type { LogicalModelGroupConfig, LogicalModelGroupsConfig } from '@/types';

const EMPTY_GROUPS: LogicalModelGroupsConfig = {
  current: { alias: 'current', ref: '' },
  static: [],
};

export function LogicalModelGroupsPage() {
  const { t } = useTranslation();
  const { showNotification } = useNotificationStore();
  const clearCache = useConfigStore((state) => state.clearCache);
  const fetchConfig = useConfigStore((state) => state.fetchConfig);

  const [groups, setGroups] = useState<LogicalModelGroupsConfig>(EMPTY_GROUPS);
  const [loading, setLoading] = useState(true);
  const [savingCurrent, setSavingCurrent] = useState(false);
  const [addingStatic, setAddingStatic] = useState(false);
  const [deletingAlias, setDeletingAlias] = useState<string | null>(null);
  const [draftCurrentRef, setDraftCurrentRef] = useState('');
  const [newAlias, setNewAlias] = useState('');
  const [newTarget, setNewTarget] = useState('');
  const [newMode, setNewMode] = useState('request');
  const [newEffort, setNewEffort] = useState('');

  const reasoningOptions = useMemo<SelectOption[]>(
    () => [
      {
        value: 'request',
        label: t('logical_model_groups.reasoning_mode_request', { defaultValue: 'Request' }),
      },
      {
        value: 'group',
        label: t('logical_model_groups.reasoning_mode_group', { defaultValue: 'Group' }),
      },
    ],
    [t]
  );

  const getReasoningModeLabel = useCallback(
    (mode?: string) => {
      return mode === 'group'
        ? t('logical_model_groups.reasoning_mode_group', { defaultValue: 'Group' })
        : t('logical_model_groups.reasoning_mode_request', { defaultValue: 'Request' });
    },
    [t]
  );

  const syncStore = useCallback(async () => {
    clearCache('logical-model-groups');
    await fetchConfig(undefined, true);
  }, [clearCache, fetchConfig]);

  const loadGroups = useCallback(async () => {
    setLoading(true);
    try {
      const data = await configApi.getLogicalModelGroups();
      setGroups(data);
      setDraftCurrentRef(data.current.ref);
    } catch (error: unknown) {
      const message =
        error instanceof Error ? error.message : typeof error === 'string' ? error : '';
      showNotification(
        `${t('notification.fetch_failed', { defaultValue: 'Failed to load data' })}${message ? `: ${message}` : ''}`,
        'error'
      );
    } finally {
      setLoading(false);
    }
  }, [showNotification, t]);

  useEffect(() => {
    void loadGroups();
  }, [loadGroups]);

  const currentDirty = useMemo(() => {
    return draftCurrentRef !== groups.current.ref;
  }, [draftCurrentRef, groups.current.ref]);

  const currentOptions = useMemo<SelectOption[]>(() => {
    return groups.static.map((group) => ({
      value: group.alias,
      label: `${group.alias} → ${group.target}`
    }));
  }, [groups.static]);

  const currentStaticGroup = useMemo(() => {
    return groups.static.find((group) => group.alias === draftCurrentRef) ?? null;
  }, [draftCurrentRef, groups.static]);

  const handleSaveCurrent = async () => {
    setSavingCurrent(true);
    try {
      await configApi.updateLogicalModelGroupCurrent(draftCurrentRef);
      await syncStore();
      await loadGroups();
      showNotification(
        t('logical_model_groups.current_saved', { defaultValue: 'Current group updated' }),
        'success'
      );
    } catch (error: unknown) {
      const message =
        error instanceof Error ? error.message : typeof error === 'string' ? error : '';
      showNotification(
        `${t('notification.update_failed')}${message ? `: ${message}` : ''}`,
        'error'
      );
    } finally {
      setSavingCurrent(false);
    }
  };

  const handleAddStatic = async () => {
    setAddingStatic(true);
    try {
      const payload: LogicalModelGroupConfig = {
        alias: newAlias,
        target: newTarget,
        reasoning: {
          mode: newMode,
          effort: newMode === 'group' ? newEffort : '',
        },
      };
      await configApi.createLogicalModelGroupStatic(payload);
      setNewAlias('');
      setNewTarget('');
      setNewMode('request');
      setNewEffort('');
      await syncStore();
      await loadGroups();
      showNotification(
        t('logical_model_groups.static_added', { defaultValue: 'Static group added' }),
        'success'
      );
    } catch (error: unknown) {
      const message =
        error instanceof Error ? error.message : typeof error === 'string' ? error : '';
      showNotification(
        `${t('notification.update_failed')}${message ? `: ${message}` : ''}`,
        'error'
      );
    } finally {
      setAddingStatic(false);
    }
  };

  const handleDeleteStatic = async (alias: string) => {
    setDeletingAlias(alias);
    try {
      await configApi.deleteLogicalModelGroupStatic(alias);
      await syncStore();
      await loadGroups();
      showNotification(
        t('logical_model_groups.static_deleted', { defaultValue: 'Static group deleted' }),
        'success'
      );
    } catch (error: unknown) {
      const message =
        error instanceof Error ? error.message : typeof error === 'string' ? error : '';
      showNotification(
        `${t('notification.update_failed')}${message ? `: ${message}` : ''}`,
        'error'
      );
    } finally {
      setDeletingAlias(null);
    }
  };

  return (
    <div style={{ display: 'grid', gap: 16 }}>
      <h1>{t('logical_model_groups.title', { defaultValue: 'Logical Model Groups' })}</h1>

      <Card title={t('logical_model_groups.current_title', { defaultValue: 'Current Group' })}>
        <p className="hint">
          {t('logical_model_groups.current_hint', {
            defaultValue: 'The fixed alias "current" points to one static group and cannot be deleted.'
          })}
        </p>
        <div style={{ display: 'grid', gap: 12 }}>
          <div className="form-group">
            <label htmlFor="logical-current-ref">
              {t('logical_model_groups.current_ref', { defaultValue: 'Static Group' })}
            </label>
            <Select
              id="logical-current-ref"
              value={draftCurrentRef}
              options={currentOptions}
              onChange={setDraftCurrentRef}
              placeholder={t('logical_model_groups.current_ref_placeholder', {
                defaultValue: 'Select a static group'
              })}
              disabled={loading || savingCurrent || currentOptions.length === 0}
            />
          </div>
          {currentStaticGroup ? (
            <div className="hint">
              {t('logical_model_groups.current_summary', {
                defaultValue: 'Target: {{target}} · Reasoning: {{mode}} {{effort}}',
                target: currentStaticGroup.target,
                mode: getReasoningModeLabel(currentStaticGroup.reasoning?.mode),
                effort: currentStaticGroup.reasoning?.effort
                  ? `(${currentStaticGroup.reasoning.effort})`
                  : ''
              })}
            </div>
          ) : (
            <div className="hint">
              {t('logical_model_groups.current_empty', {
                defaultValue: 'Create at least one static group before pointing current.'
              })}
            </div>
          )}
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 12 }}>
          <Button
            onClick={() => void handleSaveCurrent()}
            loading={savingCurrent}
            disabled={!currentDirty || !draftCurrentRef.trim()}
          >
            {t('common.save')}
          </Button>
        </div>
      </Card>

      <Card title={t('logical_model_groups.static_title', { defaultValue: 'Static Groups' })}>
        <p className="hint">
          {t('logical_model_groups.static_hint', {
            defaultValue: 'Each static group exposes one stable client-visible alias.'
          })}
        </p>
        {groups.static.length === 0 ? (
          <div className="hint">
            {t('logical_model_groups.empty', { defaultValue: 'No static groups yet.' })}
          </div>
        ) : (
          <div style={{ display: 'grid', gap: 12 }}>
            {groups.static.map((group) => (
              <div
                key={group.alias}
                style={{
                  display: 'grid',
                  gap: 8,
                  padding: 12,
                  border: '1px solid var(--border-color, rgba(127,127,127,0.25))',
                  borderRadius: 12,
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <div>
                    <div style={{ fontWeight: 600 }}>{group.alias}</div>
                    <div className="hint">{group.target}</div>
                  </div>
                  <Button
                    variant="danger"
                    size="sm"
                    loading={deletingAlias === group.alias}
                    onClick={() => void handleDeleteStatic(group.alias)}
                  >
                    {t('common.delete', { defaultValue: 'Delete' })}
                  </Button>
                </div>
                <div className="hint">
                  {t('logical_model_groups.reasoning_summary', {
                    defaultValue: 'Reasoning: {{mode}} {{effort}}',
                    mode: getReasoningModeLabel(group.reasoning?.mode),
                    effort: group.reasoning?.effort ? `(${group.reasoning.effort})` : ''
                  })}
                </div>
              </div>
            ))}
          </div>
        )}

        <div
          style={{
            display: 'grid',
            gap: 12,
            marginTop: 16,
            paddingTop: 16,
            borderTop: '1px solid var(--border-color, rgba(127,127,127,0.2))',
          }}
        >
          <Input
            label={t('logical_model_groups.alias', { defaultValue: 'Alias' })}
            value={newAlias}
            onChange={(event) => setNewAlias(event.target.value)}
            placeholder="gpt-5.4"
            disabled={addingStatic}
          />
          <Input
            label={t('logical_model_groups.target', { defaultValue: 'Target Model' })}
            value={newTarget}
            onChange={(event) => setNewTarget(event.target.value)}
            placeholder="gpt-5.4"
            disabled={addingStatic}
          />
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <div className="form-group">
              <label htmlFor="logical-new-mode">
                {t('logical_model_groups.reasoning_mode', { defaultValue: 'Reasoning Mode' })}
              </label>
              <Select
                id="logical-new-mode"
                value={newMode}
                options={reasoningOptions}
                onChange={setNewMode}
              />
            </div>
            <Input
              label={t('logical_model_groups.reasoning_effort', {
                defaultValue: 'Reasoning Effort'
              })}
              value={newEffort}
              onChange={(event) => setNewEffort(event.target.value)}
              placeholder="high"
              disabled={newMode !== 'group' || addingStatic}
            />
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <Button
              onClick={() => void handleAddStatic()}
              loading={addingStatic}
              disabled={!newAlias.trim() || !newTarget.trim()}
            >
              {t('logical_model_groups.add_static', { defaultValue: 'Add Static Group' })}
            </Button>
          </div>
        </div>
      </Card>
    </div>
  );
}
