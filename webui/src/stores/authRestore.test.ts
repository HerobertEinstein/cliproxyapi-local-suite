import { describe, expect, it } from 'vitest';

import {
  buildCleanLauncherUrl,
  extractLauncherManagementKey,
  isLauncherAutoLoginRequested,
  resolveRestoreSessionState,
} from './authRestore';

describe('authRestore', () => {
  it('uses launcher key to override stale persisted auth when launcher bootstrap is requested', () => {
    const result = resolveRestoreSessionState({
      persistedApiBase: 'http://127.0.0.1:8317',
      persistedManagementKey: 'stale-key',
      persistedRememberPassword: true,
      legacyApiBase: '',
      legacyManagementKey: '',
      launcherManagementKey: 'current-key',
      detectedApiBase: 'http://127.0.0.1:8317',
      wasLoggedIn: true,
      launcherAutoLoginRequested: true,
    });

    expect(result).toEqual({
      apiBase: 'http://127.0.0.1:8317',
      managementKey: 'current-key',
      rememberPassword: true,
      shouldAutoLogin: true,
    });
  });

  it('can recover auto login from launcher key even after the page cleared isLoggedIn state', () => {
    const result = resolveRestoreSessionState({
      persistedApiBase: '',
      persistedManagementKey: '',
      persistedRememberPassword: false,
      legacyApiBase: '',
      legacyManagementKey: '',
      launcherManagementKey: 'current-key',
      detectedApiBase: 'http://127.0.0.1:8317',
      wasLoggedIn: false,
      launcherAutoLoginRequested: true,
    });

    expect(result).toEqual({
      apiBase: 'http://127.0.0.1:8317',
      managementKey: 'current-key',
      rememberPassword: true,
      shouldAutoLogin: true,
    });
  });

  it('does not force launcher key fallback during normal page opens', () => {
    const result = resolveRestoreSessionState({
      persistedApiBase: '',
      persistedManagementKey: '',
      persistedRememberPassword: false,
      legacyApiBase: '',
      legacyManagementKey: '',
      launcherManagementKey: 'current-key',
      detectedApiBase: 'http://127.0.0.1:8317',
      wasLoggedIn: false,
      launcherAutoLoginRequested: false,
    });

    expect(result).toEqual({
      apiBase: 'http://127.0.0.1:8317',
      managementKey: '',
      rememberPassword: false,
      shouldAutoLogin: false,
    });
  });

  it('recognizes the launcher marker from the URL hash', () => {
    expect(isLauncherAutoLoginRequested('#launcher-auto-login')).toBe(true);
    expect(isLauncherAutoLoginRequested('?launcher_auto_login=1')).toBe(true);
    expect(isLauncherAutoLoginRequested('#/dashboard')).toBe(false);
  });

  it('extracts launcher key from the URL hash without exposing it to the query string flow', () => {
    expect(extractLauncherManagementKey('#launcher_key=current-key')).toBe('current-key');
    expect(extractLauncherManagementKey('#probe=1&launcher_key=second-key')).toBe('second-key');
    expect(
      extractLauncherManagementKey('#/login?launcher_auto_login=1&launcher_key=router-key')
    ).toBe('router-key');
    expect(extractLauncherManagementKey('')).toBe('');
  });

  it('removes launcher bootstrap params while preserving the hash-router route', () => {
    expect(
      buildCleanLauncherUrl(
        '/management.html',
        '',
        '#/login?launcher_auto_login=1&launcher_key=current-key&probe=1'
      )
    ).toBe('/management.html#/login?probe=1');
  });

  it('removes launcher bootstrap params from the normal query string flow', () => {
    expect(
      buildCleanLauncherUrl('/management.html', '?launcher_auto_login=1&foo=1', '')
    ).toBe('/management.html?foo=1');
  });
});
