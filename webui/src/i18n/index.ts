/**
 * i18next 国际化配置
 */

import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import zhCN from './locales/zh-CN.json';
import en from './locales/en.json';
import ru from './locales/ru.json';
import { logicalModelGroupsResources } from './logicalModelGroupsResources';
import { getInitialLanguage } from '@/utils/language';

const mergeLocale = <T extends Record<string, unknown>>(
  base: T,
  locale: keyof typeof logicalModelGroupsResources
) => {
  const extra = logicalModelGroupsResources[locale];

  return {
    ...base,
    ...extra,
    nav: {
      ...(base.nav as Record<string, unknown> | undefined),
      ...extra.nav,
    },
  };
};

i18n.use(initReactI18next).init({
  resources: {
    'zh-CN': { translation: mergeLocale(zhCN, 'zh-CN') },
    en: { translation: mergeLocale(en, 'en') },
    ru: { translation: mergeLocale(ru, 'ru') }
  },
  lng: getInitialLanguage(),
  fallbackLng: 'zh-CN',
  interpolation: {
    escapeValue: false // React 已经转义
  },
  react: {
    useSuspense: false
  }
});

export default i18n;
