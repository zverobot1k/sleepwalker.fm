'use client';

import { createContext, useContext, useMemo, useState } from 'react';
import { Language, messages } from '@/lib/messages';

type I18nContextValue = {
  lang: Language;
  setLang: (lang: Language) => void;
  t: (key: string) => string;
};

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [lang, setLangState] = useState<Language>(() => {
    if (typeof window === 'undefined') return 'en';
    const stored = localStorage.getItem('swfm_lang');
    return stored === 'ru' ? 'ru' : 'en';
  });

  const setLang = (next: Language) => {
    setLangState(next);
    localStorage.setItem('swfm_lang', next);
  };

  const value = useMemo(
    () => ({
      lang,
      setLang,
      t: (key: string) => messages[lang][key] || key,
    }),
    [lang],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error('useI18n must be used inside I18nProvider');
  }
  return ctx;
}
