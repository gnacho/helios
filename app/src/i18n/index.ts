import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { format } from 'date-fns';
import type { Locale } from 'date-fns';
import { es, enUS, zhCN } from 'date-fns/locale';

import esTranslation from './locales/es/translation.json';
import enTranslation from './locales/en/translation.json';
import zhCNTranslation from './locales/zh-CN/translation.json';

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      es: {
        translation: esTranslation,
      },
      en: {
        translation: enTranslation,
      },
      'zh-CN': {
        translation: zhCNTranslation,
      },
    },
    fallbackLng: 'es',
    interpolation: {
      escapeValue: false,
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
  });

/** Locale de date-fns según el idioma activo de i18n. */
export function dateLocale(): Locale {
  const lang = i18n.language || 'es';
  if (lang.startsWith('zh')) return zhCN;
  if (lang.startsWith('en')) return enUS;
  return es;
}

/** Tag BCP-47 para Intl.NumberFormat según el idioma activo. */
export function numLocale(): string {
  const lang = i18n.language || 'es';
  if (lang.startsWith('zh')) return 'zh-CN';
  if (lang.startsWith('en')) return 'en-US';
  return 'es-ES';
}

/** `31 jul` / `Jul 31` / `7月31日` según el idioma activo. */
export function fmtDayMonth(d: Date): string {
  const loc = dateLocale();
  if (loc === zhCN) return format(d, 'M月d日', { locale: loc });
  if (loc === enUS) return format(d, 'MMM d', { locale: loc });
  return format(d, 'd MMM', { locale: loc });
}

/** `31 de julio` / `July 31` / `7月31日` (sin año). */
export function fmtDayMonthLong(d: Date): string {
  const loc = dateLocale();
  if (loc === zhCN) return format(d, 'M月d日', { locale: loc });
  if (loc === enUS) return format(d, 'MMMM d', { locale: loc });
  return format(d, "d 'de' MMMM", { locale: loc });
}

/** `viernes, 31 de julio` / `Friday, July 31` / `7月31日 星期五`. */
export function fmtWeekdayDate(d: Date): string {
  const loc = dateLocale();
  if (loc === zhCN) return format(d, 'M月d日 EEEE', { locale: loc });
  if (loc === enUS) return format(d, 'EEEE, MMMM d', { locale: loc });
  return format(d, "EEEE, d 'de' MMMM", { locale: loc });
}

/** `julio 2026` / `July 2026` / `2026年7月`. */
export function fmtMonthYear(d: Date): string {
  const loc = dateLocale();
  if (loc === zhCN) return format(d, 'yyyy年M月', { locale: loc });
  return format(d, 'MMMM yyyy', { locale: loc });
}

export default i18n;
