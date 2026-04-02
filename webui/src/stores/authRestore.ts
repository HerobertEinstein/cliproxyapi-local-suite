import { normalizeApiBase } from '@/utils/connection';

export interface ResolveRestoreSessionInput {
  persistedApiBase: string;
  persistedManagementKey: string;
  persistedRememberPassword: boolean;
  legacyApiBase: string;
  legacyManagementKey: string;
  launcherManagementKey: string;
  detectedApiBase: string;
  wasLoggedIn: boolean;
  launcherAutoLoginRequested: boolean;
}

export interface ResolveRestoreSessionResult {
  apiBase: string;
  managementKey: string;
  rememberPassword: boolean;
  shouldAutoLogin: boolean;
}

function stripLauncherBootstrapParams(params: URLSearchParams): URLSearchParams {
  const cleaned = new URLSearchParams(params.toString());
  cleaned.delete('launcher_key');
  cleaned.delete('launcher_auto_login');
  cleaned.delete('launcher-auto-login');
  return cleaned;
}

export function isLauncherAutoLoginRequested(locationFragment: string): boolean {
  return (
    locationFragment.includes('launcher-auto-login') ||
    locationFragment.includes('launcher_auto_login=1')
  );
}

export function extractLauncherManagementKey(hash: string): string {
  const fragment = hash.startsWith('#') ? hash.slice(1) : hash;
  const params = new URLSearchParams(fragment);
  return (params.get('launcher_key') || '').trim();
}

export function buildCleanLauncherUrl(
  pathname: string,
  search: string,
  hash: string
): string {
  const cleanedSearchParams = stripLauncherBootstrapParams(
    new URLSearchParams(search.startsWith('?') ? search.slice(1) : search)
  );
  const cleanedSearch = cleanedSearchParams.toString();

  const fragment = hash.startsWith('#') ? hash.slice(1) : hash;
  let cleanedHash = '';

  if (fragment) {
    if (fragment.startsWith('/')) {
      const [routePath, routeQuery = ''] = fragment.split('?', 2);
      const cleanedRouteParams = stripLauncherBootstrapParams(new URLSearchParams(routeQuery));
      const cleanedRouteQuery = cleanedRouteParams.toString();
      cleanedHash = `${routePath}${cleanedRouteQuery ? `?${cleanedRouteQuery}` : ''}`;
    } else {
      const cleanedFragmentParams = stripLauncherBootstrapParams(new URLSearchParams(fragment));
      const cleanedFragment = cleanedFragmentParams.toString();
      cleanedHash = cleanedFragment;
    }
  }

  return `${pathname}${cleanedSearch ? `?${cleanedSearch}` : ''}${cleanedHash ? `#${cleanedHash}` : ''}`;
}

export function resolveRestoreSessionState(
  input: ResolveRestoreSessionInput
): ResolveRestoreSessionResult {
  const launcherManagementKey = input.launcherManagementKey.trim();
  const persistedManagementKey = input.persistedManagementKey.trim();
  const legacyManagementKey = input.legacyManagementKey.trim();
  const launcherBootstrapEnabled =
    input.launcherAutoLoginRequested && launcherManagementKey.length > 0;

  const apiBase = normalizeApiBase(
    input.persistedApiBase || input.legacyApiBase || input.detectedApiBase
  );
  const managementKey = launcherBootstrapEnabled
    ? launcherManagementKey
    : persistedManagementKey || legacyManagementKey;
  const rememberPassword =
    input.persistedRememberPassword ||
    persistedManagementKey.length > 0 ||
    legacyManagementKey.length > 0 ||
    launcherBootstrapEnabled;
  const shouldAutoLogin =
    Boolean(apiBase) &&
    Boolean(managementKey) &&
    (input.wasLoggedIn || launcherBootstrapEnabled);

  return {
    apiBase,
    managementKey,
    rememberPassword,
    shouldAutoLogin,
  };
}
