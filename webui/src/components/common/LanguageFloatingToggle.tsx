import { useTranslation } from 'react-i18next';
import { useLanguageStore } from '@/stores';
import type { Language } from '@/types/common';
import styles from './LanguageFloatingToggle.module.scss';

const LANGUAGE_BUTTONS: Array<{
  language: Language;
  label: string;
  title: string;
}> = [
  {
    language: 'zh-CN',
    label: '中',
    title: '切换到中文',
  },
  {
    language: 'en',
    label: 'EN',
    title: 'Switch to English',
  },
];

export function LanguageFloatingToggle() {
  const { t } = useTranslation();
  const language = useLanguageStore((state) => state.language);
  const setLanguage = useLanguageStore((state) => state.setLanguage);

  return (
    <div className={styles.toggle} role="group" aria-label={t('language.switch')}>
      {LANGUAGE_BUTTONS.map((item) => {
        const active = item.language === language;

        return (
          <button
            key={item.language}
            type="button"
            className={`${styles.button} ${active ? styles.active : ''}`}
            aria-label={item.title}
            aria-pressed={active}
            title={item.title}
            onClick={() => setLanguage(item.language)}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
}
