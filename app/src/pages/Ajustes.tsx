import { useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';
import { format } from 'date-fns';
import {
  BatteryCharging,
  Bell,
  Check,
  HeartPulse,
  House,
  KeyRound,
  LayoutGrid,
  LogOut,
  MapPin,
  Moon,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Sun,
  Sunrise,
  Trash2,
  User,
  UserPlus,
  Zap,
} from 'lucide-react';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useTheme } from '@/theme/ThemeProvider';
import type { ThemeMode } from '@/theme/ThemeProvider';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { useEnergySettings } from '@/hooks/useEnergySettings';
import { usePush } from '@/hooks/usePush';
import BrandLogo from '@/components/BrandLogo';
import { heliosToast } from '@/lib/toast';
import { LANG_MODE_KEY, resolveNavigatorLanguage, dateLocale, numLocale } from '@/i18n';
import { fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';
import { ApiError, apiDelete, apiFetch, apiPost, apiPut } from '@/data/api-client';
import pkg from '../../package.json';

const easeOutQuart = [0.25, 1, 0.5, 1] as [number, number, number, number];

const ENTIDADES: { entidad: string; descKey: string }[] = [
  { entidad: 'sensor.solis_potencia_actual', descKey: 'desc_solis_potencia_actual' },
  { entidad: 'sensor.almacen_pinza_power_b', descKey: 'desc_almacen_pinza_power_b' },
  { entidad: 'sensor.medidor_respaldo_power', descKey: 'desc_medidor_respaldo_power' },
  { entidad: 'sensor.vivienda_medidor_power', descKey: 'desc_vivienda_medidor_power' },
  { entidad: 'sensor.almacen_pinza_power_a', descKey: 'desc_almacen_pinza_power_a' },
  { entidad: 'sensor.solis_bateria_soc', descKey: 'desc_solis_bateria_soc' },
  { entidad: 'sensor.solis_bateria_potencia', descKey: 'desc_solis_bateria_potencia' },
  { entidad: 'sensor.solis_scraper', descKey: 'desc_solis_scraper' },
  { entidad: 'sun.sun', descKey: 'desc_sun' },
];

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
}

/** Sección-tarjeta con reveal al entrar en viewport. */
function Section({
  id,
  title,
  badge,
  children,
}: {
  id: string;
  title: string;
  badge?: ReactNode;
  children: ReactNode;
}) {
  return (
    <motion.section
      id={id}
      initial={{ opacity: 0, y: 24 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.15 }}
      transition={{ duration: 0.5, ease: easeOutQuart }}
      className="helios-card scroll-mt-20 p-5 shadow-card dark:shadow-card-dark lg:scroll-mt-6"
    >
      <div className="mb-4 flex items-center gap-2.5">
        <h2 className="text-[15px] font-semibold text-app">{title}</h2>
        {badge}
      </div>
      {children}
    </motion.section>
  );
}

/** Icono de salud: verde si el flujo está vivo, ámbar si no. */
function HealthBadge({ ok }: { ok: boolean }) {
  const { t } = useTranslation();
  return (
    <span
      role="status"
      aria-label={ok ? t('ajustes.health.alive') : t('ajustes.health.down')}
      title={ok ? t('ajustes.health.alive') : t('ajustes.health.down')}
      className={cn(
        'ml-auto flex h-6 w-6 items-center justify-center rounded-full',
        ok ? 'bg-emerald-500/15 text-emerald-500' : 'bg-amber-500/15 text-amber-500',
      )}
    >
      <HeartPulse size={14} strokeWidth={2.2} />
    </span>
  );
}

// ── §1 Tema ──────────────────────────────────────────────────────────────────

const THEME_OPTIONS: { value: ThemeMode; labelKey: string; icon: typeof Sun }[] = [
  { value: 'auto', labelKey: 'theme.auto', icon: Sunrise },
  { value: 'light', labelKey: 'theme.light', icon: Sun },
  { value: 'dark', labelKey: 'theme.dark', icon: Moon },
];

const THEME_PREVIEWS = [
  { key: 'light', labelKey: 'theme.light' },
  { key: 'dark', labelKey: 'theme.dark' },
] as const;

function ThemeSection() {
  const { mode, setMode, effective, density, setDensity, reduceMotion, setReduceMotion } = useTheme();
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-4">
      {/* Segmented grande */}
      <div role="radiogroup" aria-label={t('theme.label')} className="grid grid-cols-3 gap-2">
        {THEME_OPTIONS.map(({ value, labelKey, icon: Icon }) => {
          const active = mode === value;
          return (
            <button
              key={value}
              role="radio"
              aria-checked={active}
              onClick={() => setMode(value)}
              className="relative flex h-16 flex-col items-center justify-center gap-1.5 rounded-xl text-sm font-medium"
            >
              {active && (
                <motion.span
                  layoutId="ajustes-theme-pill"
                  transition={{ type: 'spring', stiffness: 400, damping: 32 }}
                  className="absolute inset-0 rounded-xl border-2 border-amber-500 bg-surface-2"
                />
              )}
              {!active && <span className="absolute inset-0 rounded-xl border border-app transition-colors hover:bg-surface-2/50" />}
              <Icon size={20} strokeWidth={2.1} className={cn('relative', active ? 'text-amber-500' : 'text-faint')} />
              <span className={cn('relative text-[13px]', active ? 'font-semibold text-app' : 'text-muted')}>{t(labelKey)}</span>
              {value === 'auto' && active && (
                <span
                  className={cn('relative h-1.5 w-1.5 rounded-full', effective === 'dark' ? 'bg-indigo-400' : 'bg-amber-500')}
                  aria-label={effective === 'dark' ? t('theme.darkActive') : t('theme.lightActive')}
                />
              )}
            </button>
          );
        })}
      </div>

      <p className="text-[13px] leading-snug text-muted">
        {t('ajustes.theme.autoNote')}
      </p>

      {/* Preview en vivo de ambos temas: tokens reales scoping .light/.dark */}
      <div className="flex flex-wrap gap-3">
        {THEME_PREVIEWS.map((tp, i) => (
          <motion.div
            key={tp.key}
            initial={{ opacity: 0, scale: 0.96 }}
            animate={{ opacity: 1, scale: 1 }}
            transition={{ duration: 0.5, delay: 0.1 + i * 0.1, ease: easeOutQuart }}
            className={cn('w-[180px] rounded-2xl border border-app bg-app p-2', tp.key)}
            aria-label={t('ajustes.theme.previewAria', { label: t(tp.labelKey) })}
          >
            <div className="flex h-[96px] flex-col justify-between rounded-xl border border-app bg-surface p-3">
              <div className="flex items-center justify-between">
                <span className="flex h-6 w-6 items-center justify-center rounded-lg bg-amber-500/15 text-amber-500">
                  <Sun size={13} strokeWidth={2.4} />
                </span>
                <span className="text-[9px] font-medium uppercase tracking-[0.08em] text-faint">
                  {t(tp.labelKey)}
                </span>
              </div>
              <div>
                <p className="text-[9px] font-medium uppercase tracking-[0.08em] text-faint">
                  {t('common.production')}
                </p>
                <p className="font-display text-lg font-semibold leading-tight text-app">
                  6,12 <span className="text-[0.6em] font-medium text-faint">kW</span>
                </p>
              </div>
            </div>
          </motion.div>
        ))}
      </div>

      {/* Densidad cómoda/compacta */}
      <div>
        <p className="text-[13px] font-medium text-muted">{t('ajustes.density.title')}</p>
        <div role="radiogroup" aria-label={t('ajustes.density.title')} className="mt-1.5 flex rounded-xl border border-app p-0.5">
          {(['comfortable', 'compact'] as const).map((d) => (
            <button
              key={d}
              role="radio"
              aria-checked={density === d}
              onClick={() => setDensity(d)}
              className={cn(
                'h-8 flex-1 rounded-lg text-[13px] transition-colors',
                density === d ? 'bg-surface-2 font-semibold text-app' : 'text-faint hover:text-muted',
              )}
            >
              {t(`ajustes.density.${d}`)}
            </button>
          ))}
        </div>
      </div>

      {/* Reducir animaciones */}
      <div className="flex items-center justify-between">
        <span className="text-sm text-app">{t('ajustes.reduceMotion')}</span>
        <button
          role="switch"
          aria-checked={reduceMotion}
          aria-label={t('ajustes.reduceMotion')}
          onClick={() => setReduceMotion(!reduceMotion)}
          className={cn('relative h-6 w-11 rounded-full transition-colors', reduceMotion ? 'bg-amber-500' : 'bg-surface-2')}
        >
          <span
            className={cn(
              'absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform',
              reduceMotion ? 'translate-x-[22px]' : 'translate-x-0.5',
            )}
          />
        </button>
      </div>
    </div>
  );
}

// ── §2 Idioma ─────────────────────────────────────────────────────────────────

function LanguageSection() {
  const { i18n, t } = useTranslation();
  const currentLanguage = i18n.language;
  const [isAuto, setIsAuto] = useState(() => localStorage.getItem(LANG_MODE_KEY) === 'auto');

  const languages = [
    { code: 'auto', name: t('ajustes.language.auto'), flag: '🌐' },
    { code: 'es', name: 'Español', flag: '🇪🇸' },
    { code: 'en', name: 'English', flag: '🇬🇧' },
    { code: 'zh-CN', name: '简体中文', flag: '🇨🇳' },
  ];

  const handleChange = (value: string) => {
    if (value === 'auto') {
      localStorage.setItem(LANG_MODE_KEY, 'auto');
      setIsAuto(true);
      i18n.changeLanguage(resolveNavigatorLanguage());
    } else {
      localStorage.setItem(LANG_MODE_KEY, 'manual');
      setIsAuto(false);
      i18n.changeLanguage(value);
      // Persistir en el perfil (BD): el idioma elegido fuerza en cualquier dispositivo.
      apiPut('/api/auth/profile', { language: value }).catch(() => {});
    }
  };

  const displayValue = isAuto ? 'auto' : currentLanguage;

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm leading-snug text-muted">
        {t('ajustes.language.note')}
      </p>
      <Select value={displayValue} onValueChange={handleChange}>
        <SelectTrigger className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {languages.map((lang) => (
            <SelectItem key={lang.code} value={lang.code}>
              <span className="flex items-center gap-2">
                <span>{lang.flag}</span>
                <span>{lang.name}</span>
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// ── §3 Conexión con Home Assistant ───────────────────────────────────────────

function ConnectionSection() {
  const { connectionStatus, getLivePower } = useEnergyData();
  const { t } = useTranslation();
  const [testing, setTesting] = useState(false);
  const live = getLivePower();
  const connected = connectionStatus === 'connected';

  const testConnection = async () => {
    if (testing) return;
    setTesting(true);
    try {
      const data = await apiFetch<{ connected?: boolean; station?: string }>('/api/solar/live');
      if (data.connected) {
        heliosToast(t('ajustes.connection.connectedToast', { station: data.station || t('ajustes.connection.stationFallback') }), { tone: 'success' });
      } else {
        heliosToast(t('ajustes.connection.unreachableToast'), { tone: 'warning' });
      }
    } catch {
      heliosToast(t('ajustes.connection.noResponseToast'), { tone: 'warning' });
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm leading-snug text-muted">
        {t('ajustes.connection.desc')}
      </p>

      <div className="flex flex-wrap items-center gap-3">
        <button
          onClick={testConnection}
          disabled={testing}
          className={cn(
            'inline-flex h-9 items-center gap-2 rounded-full px-5 text-[13px] font-semibold text-white shadow-md transition-all',
            'bg-brand-gradient hover:scale-[1.03] active:scale-95 disabled:opacity-70 disabled:hover:scale-100',
          )}
        >
          {testing && <RefreshCw size={14} className="animate-spin" />}
          {testing ? t('ajustes.connection.testing') : t('ajustes.connection.test')}
        </button>
        <span className="inline-flex h-8 items-center gap-2 rounded-full border border-app bg-surface px-3 text-xs font-medium text-muted">
          <span className="relative flex h-2 w-2">
            {connected && (
              <span className="absolute inline-flex h-full w-full animate-ping-soft rounded-full bg-emerald-500 opacity-60" />
            )}
            <span className={cn('relative inline-flex h-2 w-2 rounded-full', connected ? 'bg-emerald-500' : 'bg-amber-500')} />
          </span>
          {connected ? t('ajustes.connection.connected', { station: live.station || 'Home Assistant' }) : t('ajustes.connection.reconnecting')}
        </span>
      </div>

      {/* Entidades leídas (colapsable) */}
      <Accordion type="single" collapsible>
        <AccordionItem value="entidades" className="rounded-xl border border-app px-3">
          <AccordionTrigger className="py-3 text-[13px] font-medium text-muted hover:no-underline">
            {t('ajustes.connection.entities')}
          </AccordionTrigger>
          <AccordionContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-xs">{t('ajustes.connection.entity')}</TableHead>
                  <TableHead className="text-xs">{t('ajustes.connection.description')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {ENTIDADES.map((e) => (
                  <TableRow key={e.entidad}>
                    <TableCell className="py-2 font-mono text-xs text-app">{e.entidad}</TableCell>
                    <TableCell className="py-2 text-xs text-muted">{t(`ajustes.connection.${e.descKey}`)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </AccordionContent>
        </AccordionItem>
      </Accordion>
    </div>
  );
}

// ── §4 Instalación ───────────────────────────────────────────────────────────

type Municipio = [string, string, number, number];

const normText = (s: string) =>
  s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase();

function LocationField() {
  const [settings, update] = useEnergySettings();
  const { sunriseMin, sunsetMin } = useEnergyData();
  const { t } = useTranslation();
  const [query, setQuery] = useState(settings.location);
  const [list, setList] = useState<Municipio[] | null>(null);
  const [open, setOpen] = useState(false);

  const load = async () => {
    if (list !== null) return;
    try {
      const res = await fetch('/municipios.json');
      setList((await res.json()) as Municipio[]);
    } catch {
      setList([]);
    }
  };

  const results = useMemo(() => {
    if (!list || query.trim().length < 2) return [];
    const q = normText(query.trim());
    const starts: Municipio[] = [];
    const contains: Municipio[] = [];
    for (const m of list) {
      const n = normText(m[0]);
      if (n.startsWith(q)) starts.push(m);
      else if (n.includes(q)) contains.push(m);
      if (starts.length >= 8) break;
    }
    return [...starts, ...contains].slice(0, 8);
  }, [list, query]);

  const select = (m: Municipio) => {
    update({ location: m[1] ? `${m[0]} (${m[1]})` : m[0], locationLat: m[2], locationLon: m[3] });
    setQuery(m[0]);
    setOpen(false);
  };

  const clear = () => {
    update({ location: '', locationLat: null, locationLon: null });
    setQuery('');
  };

  return (
    <div>
      <Label htmlFor="install-location" className="mb-1.5 block text-[13px] font-medium text-muted">
        {t('ajustes.install.location')}
      </Label>
      <div className="relative">
        <MapPin size={15} className="pointer-events-none absolute left-3 top-1/2 z-10 -translate-y-1/2 text-faint" />
        <Input
          id="install-location"
          value={query}
          placeholder={t('ajustes.install.locationPlaceholder')}
          autoComplete="off"
          onChange={(e) => {
            setQuery(e.target.value);
            setOpen(true);
          }}
          onFocus={() => {
            void load();
            setOpen(true);
          }}
          onBlur={() => window.setTimeout(() => setOpen(false), 150)}
          className="pl-9 pr-9"
        />
        {settings.locationLat !== null && (
          <button
            type="button"
            onClick={clear}
            aria-label={t('ajustes.install.clearLocation')}
            title={t('ajustes.install.clearLocation')}
            className="absolute right-2.5 top-1/2 -translate-y-1/2 text-faint transition-colors hover:text-muted"
          >
            ✕
          </button>
        )}
        {open && results.length > 0 && (
          <ul className="absolute left-0 right-0 top-full z-20 mt-1 max-h-64 overflow-auto rounded-xl border border-app bg-surface py-1 shadow-lg">
            {results.map((m, i) => (
              <li key={`${m[0]}-${i}`}>
                <button
                  type="button"
                  onMouseDown={(e) => {
                    e.preventDefault();
                    select(m);
                  }}
                  className="flex w-full items-baseline gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-surface-2"
                >
                  <span className="font-medium text-app">{m[0]}</span>
                  <span className="truncate text-xs text-faint">{m[1]}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
      <p className="mt-1.5 text-xs text-faint">
        {settings.locationLat !== null
          ? t('ajustes.install.sunTodayAt', { location: settings.location.split(' (')[0], sunrise: fmtTime(sunriseMin), sunset: fmtTime(sunsetMin) })
          : t('ajustes.install.sunTodayHa', { sunrise: fmtTime(sunriseMin), sunset: fmtTime(sunsetMin) })}
      </p>
    </div>
  );
}

const INVERTERS = [
  {
    key: 'solis',
    name: 'Solis S5-EH1P5K-L',
    color: 'var(--c-solis)',
    panels: 10,
    panelW: 440,
    kwp: 4.4,
    battery: { name: 'Soluna', kwh: 5 },
  },
  {
    key: 'fox',
    name: 'Fox H1-3.0-E',
    color: 'var(--c-fox)',
    panels: 6,
    panelW: 450,
    kwp: 2.7,
    battery: null,
  },
] as const;

const TOTAL_KWP = INVERTERS.reduce((acc, inv) => acc + inv.kwp, 0);

const fmtKwp = (v: number) => new Intl.NumberFormat(numLocale(), { maximumFractionDigits: 1 }).format(v);

function InstallationSection() {
  const [settings, update] = useEnergySettings();
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 md:grid-cols-2">
        {INVERTERS.map((inv, i) => (
          <motion.div
            key={inv.key}
            initial={{ opacity: 0, x: i === 0 ? -24 : 24 }}
            whileInView={{ opacity: 1, x: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.5, delay: i * 0.12, ease: easeOutQuart }}
            className="group rounded-xl border-2 bg-surface p-4 transition-colors"
            style={{ borderColor: `color-mix(in srgb, ${inv.color} 35%, transparent)` }}
          >
            <div className="mb-3 flex items-center gap-2">
              <span
                className="flex h-8 w-8 items-center justify-center rounded-lg transition-transform group-hover:scale-110"
                style={{ backgroundColor: `color-mix(in srgb, ${inv.color} 12%, transparent)`, color: inv.color }}
              >
                <Zap size={16} strokeWidth={2.2} />
              </span>
              <p className="font-display text-[15px] font-semibold text-app">{inv.name}</p>
            </div>

            {/* Capacidad teórica del inversor, en grande */}
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.install.theoretical')}</p>
            <p className="font-display text-3xl font-bold leading-tight text-app">
              {fmtKwp(inv.kwp)} <span className="text-base font-medium text-faint">kWp</span>
            </p>

            <div className="mt-3 flex flex-col gap-2">
              <p className="flex items-center gap-2 text-[13px] text-muted">
                <LayoutGrid size={15} className="shrink-0 text-faint" />
                {t('ajustes.install.panels', { count: inv.panels, watts: inv.panelW })}
              </p>
              {inv.battery && (
                <p className="flex items-center gap-2 text-[13px] text-muted">
                  <BatteryCharging size={15} className="shrink-0 text-faint" />
                  {t('ajustes.install.battery', { name: inv.battery.name, kwh: inv.battery.kwh })}
                </p>
              )}
            </div>
          </motion.div>
        ))}
      </div>

      {/* Capacidad teórica TOTAL, en grande */}
      <motion.div
        initial={{ opacity: 0, y: 16 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true }}
        transition={{ duration: 0.5, delay: 0.2, ease: easeOutQuart }}
        className="flex items-center justify-center gap-4 rounded-2xl border border-app bg-surface-2 px-6 py-5"
      >
        <Sun size={30} className="shrink-0 text-amber-500" />
        <div className="text-center">
          <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.install.totalLabel')}</p>
          <p className="font-display text-4xl font-bold leading-tight text-app">
            {fmtKwp(TOTAL_KWP)} <span className="text-lg font-medium text-faint">kWp</span>
          </p>
        </div>
      </motion.div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <Label htmlFor="install-name" className="mb-1.5 block text-[13px] font-medium text-muted">
            {t('ajustes.install.name')}
          </Label>
          <div className="relative">
            <House size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint" />
            <Input
              id="install-name"
              value={settings.installName}
              onChange={(e) => update({ installName: e.target.value })}
              className="pl-9"
            />
          </div>
        </div>
        <LocationField />
      </div>
    </div>
  );
}

// ── §5 Precios y ahorro ──────────────────────────────────────────────────────

function toEsDecimal(v: number, decimals = 2): string {
  return v.toFixed(decimals).replace('.', ',');
}

function fromEsDecimal(v: string): number | null {
  const n = Number(v.trim().replace(',', '.'));
  return Number.isFinite(n) && v.trim() !== '' ? n : null;
}

function PricesSection() {
  const [settings, update] = useEnergySettings();
  const { t } = useTranslation();
  const [priceImport, setPriceImport] = useState(() => toEsDecimal(settings.priceImport));
  const [priceExport, setPriceExport] = useState(() => toEsDecimal(settings.priceExport));
  const [co2, setCo2] = useState(() => toEsDecimal(settings.co2Factor));

  // Persiste lo escrito (antes solo vivía en estado local y el Guardar global hacía update({})).
  const persist = () => {
    const pi = fromEsDecimal(priceImport);
    const pe = fromEsDecimal(priceExport);
    const c = fromEsDecimal(co2);
    update({
      ...(pi !== null ? { priceImport: pi } : {}),
      ...(pe !== null ? { priceExport: pe } : {}),
      ...(c !== null ? { co2Factor: c } : {}),
    });
  };

  const fields = [
    { id: 'precio-compra', label: t('ajustes.prices.buy'), value: priceImport, set: setPriceImport, suffix: '€/kWh' },
    { id: 'precio-vertido', label: t('ajustes.prices.sell'), value: priceExport, set: setPriceExport, suffix: '€/kWh' },
    { id: 'factor-co2', label: t('ajustes.prices.co2'), value: co2, set: setCo2, suffix: 'kg/kWh' },
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 sm:grid-cols-3">
        {fields.map((f, i) => (
          <motion.div
            key={f.id}
            initial={{ opacity: 0, y: 16 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.4, delay: i * 0.08, ease: 'easeOut' }}
          >
            <Label htmlFor={f.id} className="mb-1.5 block text-[13px] font-medium text-muted">
              {f.label}
            </Label>
            <div className="relative">
              <Input
                id={f.id}
                inputMode="decimal"
                value={f.value}
                onChange={(e) => f.set(e.target.value)}
                onBlur={persist}
                className="pr-16 text-right tabular-nums"
              />
              <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs font-medium text-faint">
                {f.suffix}
              </span>
            </div>
          </motion.div>
        ))}
      </div>

      <p className="text-[13px] leading-snug text-muted">
        {t('ajustes.prices.note')}
      </p>
    </div>
  );
}

// ── §6 Datos ─────────────────────────────────────────────────────────────────

function DataSection() {
  const { liveTick } = useEnergyData();
  const { t } = useTranslation();
  const [lastSyncAt, setLastSyncAt] = useState(() => Date.now());
  const [nowTs, setNowTs] = useState(() => Date.now());

  const liveTickRef = useRef(liveTick);
  useEffect(() => {
    const id = window.setInterval(() => {
      if (liveTickRef.current !== liveTick) {
        liveTickRef.current = liveTick;
        setLastSyncAt(Date.now());
      }
      setNowTs(Date.now());
    }, 1000);
    return () => window.clearInterval(id);
  }, [liveTick]);

  const secs = Math.max(0, Math.round((nowTs - lastSyncAt) / 1000));

  return (
    <div className="flex flex-col gap-4">
      <div>
        <p className="text-sm font-medium text-app">{t('ajustes.data.title')}</p>
        <p className="mt-0.5 max-w-md text-[13px] leading-snug text-muted">
          {t('ajustes.data.desc')}
        </p>
      </div>

      <div className="flex items-center gap-2 border-t border-app pt-3 text-[13px] text-muted">
        <span>
          {t('ajustes.data.lastUpdate', { secs })}
        </span>
      </div>
    </div>
  );
}

// ── §7 Seguridad (contraseña propia) ────────────────────────────────────────

function SecuritySection() {
  const { t } = useTranslation();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const mismatch = confirm.length > 0 && next !== confirm;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy || mismatch) return;
    setBusy(true);
    setError(null);
    try {
      await apiPut('/api/auth/password', { current, password: next });
      heliosToast(t('ajustes.security.changed'), { tone: 'success' });
      setCurrent('');
      setNext('');
      setConfirm('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('common.error'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <p className="text-sm leading-snug text-muted">{t('ajustes.security.desc')}</p>
      <div>
        <Label htmlFor="sec-current" className="mb-1.5 block text-[13px] font-medium text-muted">
          {t('ajustes.security.current')}
        </Label>
        <Input id="sec-current" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <Label htmlFor="sec-new" className="mb-1.5 block text-[13px] font-medium text-muted">
            {t('ajustes.security.new')}
          </Label>
          <Input id="sec-new" type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="sec-confirm" className="mb-1.5 block text-[13px] font-medium text-muted">
            {t('ajustes.security.confirm')}
          </Label>
          <Input id="sec-confirm" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
        </div>
      </div>
      {mismatch && (
        <p className="rounded-lg bg-rose-500/10 px-3 py-2 text-center text-[13px] font-medium text-rose-600 dark:text-rose-400" role="alert">
          {t('ajustes.security.mismatch')}
        </p>
      )}
      {error && (
        <p className="rounded-lg bg-rose-500/10 px-3 py-2 text-center text-[13px] font-medium text-rose-600 dark:text-rose-400" role="alert">
          {error}
        </p>
      )}
      <button
        type="submit"
        disabled={busy || !current || next.length < 6 || next !== confirm}
        className="bg-brand-gradient inline-flex h-10 items-center justify-center gap-2 rounded-full text-sm font-semibold text-white shadow-md transition-transform hover:scale-[1.02] active:scale-95 disabled:opacity-60 disabled:hover:scale-100"
      >
        <KeyRound size={16} />
        {t('ajustes.security.submit')}
      </button>
    </form>
  );
}

// ── §7b Mi sesión (patrón easyzfs: cambiar contraseña + cerrar sesión) ──────

async function doLogout() {
  // Contrato de sesión: logout = POST + evento unauthorized (AuthGate muestra Login). Sin recargar la SPA.
  try {
    await apiPost('/api/auth/logout');
  } finally {
    window.dispatchEvent(new Event('helios-unauthorized'));
  }
}

function SessionSection() {
  const { t } = useTranslation();
  const [showPwd, setShowPwd] = useState(false);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap gap-2.5">
        <button
          onClick={() => setShowPwd((v) => !v)}
          aria-expanded={showPwd}
          className="inline-flex h-10 items-center gap-2 rounded-full border border-app bg-surface px-5 text-sm font-semibold text-app transition-colors hover:bg-surface-2"
        >
          <KeyRound size={16} />
          {t('ajustes.security.change')}
        </button>
        <button
          onClick={doLogout}
          className="inline-flex h-10 items-center gap-2 rounded-full border border-destructive/30 bg-destructive/10 px-5 text-sm font-semibold text-destructive transition-colors hover:bg-destructive/20"
        >
          <LogOut size={16} />
          {t('common.logout')}
        </button>
      </div>
      <AnimatePresence>
        {showPwd && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            className="overflow-hidden"
          >
            <SecuritySection />
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

// ── §8 Usuarios (solo admin) ────────────────────────────────────────────────

interface AdminUser {
  id: string;
  username: string;
  email?: string | null;
  phone?: string | null;
  language: string;
  role: string;
}

function UsersSection() {
  const { t } = useTranslation();
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [meId, setMeId] = useState<string | null>(null);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [language, setLanguage] = useState('es');
  const [role, setRole] = useState<'user' | 'admin'>('user');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pwdFor, setPwdFor] = useState<string | null>(null);
  const [newPwd, setNewPwd] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  const languages = [
    { code: 'es', name: 'Español', flag: '🇪🇸' },
    { code: 'en', name: 'English', flag: '🇬🇧' },
    { code: 'zh-CN', name: '简体中文', flag: '🇨🇳' },
  ];

  const reload = () => {
    apiFetch<{ users?: AdminUser[] }>('/api/auth/users')
      .then((data) => setUsers(data.users || []))
      .catch(() => setUsers([]));
  };

  useEffect(() => {
    reload();
    apiFetch<{ authenticated?: boolean; user?: { id: string } }>('/api/auth/me')
      .then((data) => {
        if (data.authenticated && data.user) setMeId(data.user.id);
      })
      .catch(() => setMeId(null));
  }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await apiPost('/api/auth/register', { username, password, language, role });
      heliosToast(t('admin.users.createdToast', { username }), { tone: 'success' });
      setUsername('');
      setPassword('');
      setLanguage('es');
      setRole('user');
      setShowCreate(false);
      reload();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('common.error'));
    } finally {
      setBusy(false);
    }
  };

  const changePassword = async (userId: string) => {
    if (newPwd.length < 6) return;
    try {
      await apiPut(`/api/auth/users/${userId}/password`, { password: newPwd });
      heliosToast(t('admin.users.passwordChanged'), { tone: 'success' });
      setPwdFor(null);
      setNewPwd('');
    } catch {
      heliosToast(t('common.error'), { tone: 'warning' });
    }
  };

  const changeLanguage = async (userId: string, lang: string) => {
    try {
      await apiPut(`/api/auth/users/${userId}/language`, { language: lang });
      heliosToast(t('admin.users.languageChanged'), { tone: 'success' });
      setUsers((us) => us.map((u) => (u.id === userId ? { ...u, language: lang } : u)));
    } catch {
      heliosToast(t('common.error'), { tone: 'warning' });
    }
  };

  const changeRole = async (userId: string, newRole: string) => {
    try {
      await apiPut(`/api/auth/users/${userId}/role`, { role: newRole });
      heliosToast(t('admin.users.roleChanged'), { tone: 'success' });
      setUsers((us) => us.map((u) => (u.id === userId ? { ...u, role: newRole } : u)));
    } catch {
      heliosToast(t('common.error'), { tone: 'warning' });
    }
  };

  const deleteUser = async (userId: string) => {
    try {
      await apiDelete(`/api/auth/users/${userId}`);
      heliosToast(t('admin.users.deleted'), { tone: 'success' });
      setConfirmDelete(null);
      setUsers((us) => us.filter((u) => u.id !== userId));
    } catch {
      heliosToast(t('common.error'), { tone: 'warning' });
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm leading-snug text-muted">
        {t('admin.users.subtitle')}
      </p>

      {/* Lista de usuarios */}
      <div className="flex flex-col gap-2">
        {users.length === 0 ? (
          <p className="text-sm text-faint">{t('admin.users.empty')}</p>
        ) : (
          users.map((user) => (
            <div key={user.id} className="rounded-lg border border-app bg-surface px-4 py-3">
              <div className="flex flex-wrap items-center gap-3">
                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface-2">
                  <User size={16} />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-app">
                    {user.username}
                    {user.id === meId && <span className="ml-1.5 text-xs font-normal text-faint">({t('admin.users.you')})</span>}
                  </p>
                  <p className="text-xs text-faint">{user.role === 'admin' ? t('admin.users.roleAdmin') : t('admin.users.roleUser')}</p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {/* Idioma */}
                  <Select value={user.language} onValueChange={(v) => changeLanguage(user.id, v)}>
                    <SelectTrigger className="h-8 w-[130px] text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {languages.map((lang) => (
                        <SelectItem key={lang.code} value={lang.code}>
                          <span className="flex items-center gap-2">
                            <span>{lang.flag}</span>
                            <span>{lang.name}</span>
                          </span>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {/* Rol (no editable sobre uno mismo) */}
                  {user.id !== meId && (
                    <Select value={user.role} onValueChange={(v) => changeRole(user.id, v)}>
                      <SelectTrigger className="h-8 w-[120px] text-xs">
                        <ShieldCheck size={13} className="mr-1 shrink-0" />
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="user">{t('admin.users.roleUser')}</SelectItem>
                        <SelectItem value="admin">{t('admin.users.roleAdmin')}</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                  {/* Contraseña */}
                  <button
                    onClick={() => {
                      setPwdFor(pwdFor === user.id ? null : user.id);
                      setNewPwd('');
                    }}
                    title={t('admin.users.changePassword')}
                    aria-label={t('admin.users.changePassword')}
                    className="flex h-8 w-8 items-center justify-center rounded-lg border border-app text-muted transition-colors hover:bg-surface-2 hover:text-app"
                  >
                    <KeyRound size={14} />
                  </button>
                  {/* Eliminar (no sobre uno mismo): confirmación inline en dos pasos */}
                  {user.id !== meId &&
                    (confirmDelete === user.id ? (
                      <span className="flex gap-1">
                        <button
                          onClick={() => deleteUser(user.id)}
                          className="h-8 rounded-lg bg-destructive px-3 text-xs font-semibold text-white transition-opacity hover:opacity-90"
                        >
                          {t('common.confirm')}
                        </button>
                        <button
                          onClick={() => setConfirmDelete(null)}
                          className="h-8 rounded-lg border border-app px-3 text-xs font-medium text-muted transition-colors hover:text-app"
                        >
                          {t('common.cancel')}
                        </button>
                      </span>
                    ) : (
                      <button
                        onClick={() => setConfirmDelete(user.id)}
                        title={t('admin.users.delete')}
                        aria-label={t('admin.users.delete')}
                        className="flex h-8 w-8 items-center justify-center rounded-lg border border-destructive/30 text-destructive transition-colors hover:bg-destructive/10"
                      >
                        <Trash2 size={14} />
                      </button>
                    ))}
                </div>
              </div>
              {/* Form inline cambio de contraseña */}
              {pwdFor === user.id && (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: 'auto' }}
                  className="mt-3 flex items-center gap-2 overflow-hidden border-t border-app pt-3"
                >
                  <Input
                    type="password"
                    autoComplete="new-password"
                    placeholder={t('admin.users.newPassword')}
                    value={newPwd}
                    onChange={(e) => setNewPwd(e.target.value)}
                    className="h-8 flex-1 text-sm"
                  />
                  <button
                    onClick={() => changePassword(user.id)}
                    disabled={newPwd.length < 6}
                    className="bg-brand-gradient h-8 rounded-full px-4 text-xs font-semibold text-white transition-transform hover:scale-[1.03] active:scale-95 disabled:opacity-60"
                  >
                    {t('common.save')}
                  </button>
                </motion.div>
              )}
            </div>
          ))
        )}
      </div>

      {/* Formulario crear usuario: oculto por defecto, se despliega al pulsar "Crear usuario" */}
      {!showCreate ? (
        <button
          type="button"
          onClick={() => setShowCreate(true)}
          className="bg-brand-gradient inline-flex h-10 items-center justify-center gap-2 self-start rounded-full px-6 text-sm font-semibold text-white shadow-md transition-transform hover:scale-[1.02] active:scale-95"
        >
          <UserPlus size={16} />
          {t('admin.users.create')}
        </button>
      ) : (
      <motion.form
        initial={{ opacity: 0, height: 0 }}
        animate={{ opacity: 1, height: 'auto' }}
        onSubmit={submit}
        className="flex flex-col gap-4 overflow-hidden border-t border-app pt-4"
      >
        <h3 className="text-sm font-semibold text-app">{t('admin.users.newUser')}</h3>
        <div>
          <Label htmlFor="admin-user" className="mb-1.5 block text-[13px] font-medium text-muted">
            {t('admin.users.username')}
          </Label>
          <div className="relative">
            <User size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint" />
            <Input
              id="admin-user"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="pl-9"
            />
          </div>
        </div>

        <div>
          <Label htmlFor="admin-pass" className="mb-1.5 block text-[13px] font-medium text-muted">
            {t('admin.users.password')}
          </Label>
          <div className="relative">
            <KeyRound size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint" />
            <Input
              id="admin-pass"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="pl-9"
            />
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <Label htmlFor="admin-lang" className="mb-1.5 block text-[13px] font-medium text-muted">
              {t('admin.users.language')}
            </Label>
            <Select value={language} onValueChange={setLanguage}>
              <SelectTrigger id="admin-lang" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {languages.map((lang) => (
                  <SelectItem key={lang.code} value={lang.code}>
                    <span className="flex items-center gap-2">
                      <span>{lang.flag}</span>
                      <span>{lang.name}</span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label htmlFor="admin-role" className="mb-1.5 block text-[13px] font-medium text-muted">
              {t('admin.users.role')}
            </Label>
            <Select value={role} onValueChange={(v) => setRole(v as 'user' | 'admin')}>
              <SelectTrigger id="admin-role" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="user">{t('admin.users.roleUser')}</SelectItem>
                <SelectItem value="admin">{t('admin.users.roleAdmin')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {error && (
          <motion.p
            initial={{ opacity: 0, y: -4 }}
            animate={{ opacity: 1, y: 0 }}
            className="rounded-lg bg-rose-500/10 px-3 py-2 text-center text-[13px] font-medium text-rose-600 dark:text-rose-400"
            role="alert"
          >
            {error}
          </motion.p>
        )}

        <div className="flex gap-2">
          <button
            type="submit"
            disabled={busy || !username || !password}
            className="bg-brand-gradient inline-flex h-10 flex-1 items-center justify-center gap-2 rounded-full text-sm font-semibold text-white shadow-md transition-transform hover:scale-[1.02] active:scale-95 disabled:opacity-60 disabled:hover:scale-100"
          >
            <UserPlus size={16} />
            {busy ? t('admin.users.creating') : t('admin.users.create')}
          </button>
          <button
            type="button"
            onClick={() => {
              setShowCreate(false);
              setError(null);
            }}
            className="inline-flex h-10 items-center justify-center rounded-full border border-app px-6 text-sm font-semibold text-muted transition-colors hover:bg-surface-2 hover:text-app"
          >
            {t('common.cancel')}
          </button>
        </div>
      </motion.form>
      )}

      {/* Actividad (audit log) */}
      <div className="border-t border-app pt-4">
        <ActivitySection />
      </div>
    </div>
  );
}

// ── §8b Actividad (audit log, solo admin) ───────────────────────────────────

interface AuditEntry {
  id: number;
  ts: number;
  actor: string;
  action: string;
  detail: string | null;
}

const AUDIT_PAGE = 20;

function ActivitySection() {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loaded, setLoaded] = useState(false);

  const load = (off: number) => {
    apiFetch<{ total: number; entries: AuditEntry[] }>(`/api/auth/audit?limit=${AUDIT_PAGE}&offset=${off}`)
      .then((data) => {
        setEntries(data.entries);
        setTotal(data.total);
        setOffset(off);
        setLoaded(true);
      })
      .catch(() => setLoaded(true));
  };

  const pages = Math.max(1, Math.ceil(total / AUDIT_PAGE));
  const page = Math.floor(offset / AUDIT_PAGE) + 1;

  return (
    <Accordion
      type="single"
      collapsible
      onValueChange={(v) => {
        if (v === 'actividad' && !loaded) load(0);
      }}
    >
      <AccordionItem value="actividad" className="rounded-xl border border-app px-3">
        <AccordionTrigger className="py-3 text-[13px] font-medium text-muted hover:no-underline">
          {t('admin.activity.title')}
        </AccordionTrigger>
        <AccordionContent>
          {entries.length === 0 ? (
            <p className="py-2 text-sm text-faint">{t('admin.activity.empty')}</p>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">{t('admin.activity.when')}</TableHead>
                    <TableHead className="text-xs">{t('admin.activity.user')}</TableHead>
                    <TableHead className="text-xs">{t('admin.activity.action')}</TableHead>
                    <TableHead className="text-xs">{t('admin.activity.detail')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((e) => (
                    <TableRow key={e.id}>
                      <TableCell className="py-1.5 text-xs whitespace-nowrap text-muted">
                        {format(new Date(e.ts), 'P p', { locale: dateLocale() })}
                      </TableCell>
                      <TableCell className="py-1.5 text-xs text-app">{e.actor}</TableCell>
                      <TableCell className="py-1.5 font-mono text-xs text-muted">{e.action}</TableCell>
                      <TableCell className="max-w-[180px] truncate py-1.5 font-mono text-[11px] text-faint">{e.detail ?? '—'}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              {pages > 1 && (
                <div className="mt-2 flex items-center justify-between text-xs text-muted">
                  <button
                    disabled={offset === 0}
                    onClick={() => load(Math.max(0, offset - AUDIT_PAGE))}
                    className="rounded-full border border-app px-3 py-1 disabled:opacity-40"
                  >
                    {t('admin.activity.prev')}
                  </button>
                  <span>
                    {page} / {pages}
                  </span>
                  <button
                    disabled={offset + AUDIT_PAGE >= total}
                    onClick={() => load(offset + AUDIT_PAGE)}
                    className="rounded-full border border-app px-3 py-1 disabled:opacity-40"
                  >
                    {t('admin.activity.next')}
                  </button>
                </div>
              )}
            </>
          )}
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  );
}

// ── §9 Instalar como app (PWA) ───────────────────────────────────────────────

type InstallState = 'installed' | 'installable' | 'ios' | 'hidden';

function useInstallPrompt() {
  const [deferred, setDeferred] = useState<BeforeInstallPromptEvent | null>(null);
  const [installed, setInstalled] = useState(
    () =>
      window.matchMedia('(display-mode: standalone)').matches ||
      (navigator as { standalone?: boolean }).standalone === true,
  );
  const isIos = /iphone|ipad|ipod/i.test(navigator.userAgent);

  useEffect(() => {
    const onPrompt = (e: Event) => {
      e.preventDefault();
      setDeferred(e as BeforeInstallPromptEvent);
    };
    const onInstalled = () => {
      setInstalled(true);
      setDeferred(null);
    };
    window.addEventListener('beforeinstallprompt', onPrompt);
    window.addEventListener('appinstalled', onInstalled);
    return () => {
      window.removeEventListener('beforeinstallprompt', onPrompt);
      window.removeEventListener('appinstalled', onInstalled);
    };
  }, []);

  // Sin evento y no-iOS: el navegador no lo soporta → la tarjeta no se muestra.
  const state: InstallState = installed ? 'installed' : deferred ? 'installable' : isIos ? 'ios' : 'hidden';

  const install = async () => {
    if (!deferred) return;
    await deferred.prompt();
    setDeferred(null);
  };

  return { state, install };
}

function InstallSection({ state, install }: { state: InstallState; install: () => void }) {
  const { t } = useTranslation();
  const { isDark } = useTheme();
  const steps = [t('ajustes.pwa.step1'), t('ajustes.pwa.step2'), t('ajustes.pwa.step3')];

  return (
    <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
      <motion.img
        src={isDark ? '/icons/oscuro/helios-icon-512.png' : '/icons/claro/helios-icon-512.png'}
        alt={t('ajustes.pwa.iconAlt')}
        initial={{ scale: 0.8, opacity: 0 }}
        whileInView={{ scale: 1, opacity: 1 }}
        viewport={{ once: true }}
        transition={{ type: 'spring', stiffness: 260, damping: 18 }}
        className="h-24 w-24 shrink-0 rounded-[22px] shadow-lg"
      />
      <div className="min-w-0 flex-1">
        <p className="font-display text-[15px] font-semibold text-app">{t('ajustes.pwa.name')}</p>
        <p className="mt-1 text-sm leading-snug text-muted">
          {t('ajustes.pwa.desc')}
        </p>
        {state === 'ios' && (
          <ol className="mt-3 flex flex-col gap-1.5">
            {steps.map((step, i) => (
              <motion.li
                key={step}
                initial={{ opacity: 0, y: 12 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true }}
                transition={{ duration: 0.4, delay: 0.15 + i * 0.1, ease: 'easeOut' }}
                className="flex items-baseline gap-2 text-sm text-muted"
              >
                <span className="flex h-5 w-5 shrink-0 translate-y-0.5 items-center justify-center rounded-full bg-surface-2 text-[11px] font-semibold text-muted">
                  {i + 1}
                </span>
                {step}
              </motion.li>
            ))}
          </ol>
        )}
        <div className="mt-6 flex justify-center">
          {state === 'installable' && (
            <button
              onClick={install}
              className="bg-brand-gradient inline-flex h-14 items-center gap-3 rounded-full px-8 text-[16px] font-semibold text-white shadow-lg transition-transform hover:scale-[1.03] active:scale-95"
            >
              <Smartphone size={22} />
              {t('ajustes.pwa.install')}
            </button>
          )}
          {state === 'installed' && (
            <p className="inline-flex items-center gap-2 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-6 py-3 text-sm font-medium text-emerald-600 dark:text-emerald-400">
              <Check size={18} />
              {t('ajustes.pwa.installed')}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

// ── §9 Acerca de ─────────────────────────────────────────────────────────────

// ── § Notificaciones push ────────────────────────────────────────────────────

const TIPOS_NOTIF = [
  'inversor_offline',
  'inversor_ok',
  'fox_offline',
  'fox_ok',
  'corte_red',
  'corte_red_ok',
  'bateria_baja',
  'resumen_diario',
] as const;

function NotificationsSection() {
  const { t } = useTranslation();
  const { soporte, estado, activar, desactivar } = usePush();
  const [prefs, setPrefs] = useState<Record<string, boolean> | null>(null);

  useEffect(() => {
    if (!estado.suscrito) {
      setPrefs(null);
      return;
    }
    apiFetch<{ prefs: Record<string, boolean> }>('/api/push/preferences')
      .then((r) => setPrefs(r.prefs))
      .catch(() => setPrefs(null));
  }, [estado.suscrito]);

  const cambiarPref = (tipo: string, enabled: boolean) => {
    setPrefs((p) => (p ? { ...p, [tipo]: enabled } : p));
    apiPut('/api/push/preferences', { tipo, enabled }).catch(() => {
      setPrefs((p) => (p ? { ...p, [tipo]: !enabled } : p)); // rollback optimista
    });
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-[13px] leading-snug text-muted">{t('ajustes.notif.descripcion')}</p>

      {estado.cargando ? (
        <div className="h-10 animate-pulse rounded-xl bg-surface-2" />
      ) : soporte === 'requiere-https' ? (
        <p className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-3.5 py-2.5 text-[13px] text-amber-600 dark:text-amber-400">
          {t('ajustes.notif.requiereHttps')}
        </p>
      ) : soporte === 'ios-necesita-instalacion' ? (
        <p className="rounded-xl border border-app bg-surface-2 px-3.5 py-2.5 text-[13px] text-muted">
          {t('ajustes.notif.iosInstalacion')}
        </p>
      ) : soporte === 'no-configurado' ? (
        <p className="rounded-xl border border-app bg-surface-2 px-3.5 py-2.5 text-[13px] text-muted">
          {t('ajustes.notif.noConfigurado')}
        </p>
      ) : soporte === 'no-soportado' ? (
        <p className="rounded-xl border border-app bg-surface-2 px-3.5 py-2.5 text-[13px] text-muted">
          {t('ajustes.notif.noSoportado')}
        </p>
      ) : (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center gap-2.5">
            {estado.suscrito ? (
              <button
                onClick={() => desactivar()}
                disabled={estado.cargando}
                className="inline-flex h-10 items-center gap-2 rounded-full border border-app bg-surface px-5 text-sm font-semibold text-app transition-colors hover:bg-surface-2 disabled:opacity-50"
              >
                <Bell size={16} />
                {t('ajustes.notif.desactivar')}
              </button>
            ) : (
              <button
                onClick={() => activar()}
                disabled={estado.cargando}
                className="inline-flex h-10 items-center gap-2 rounded-full border border-amber-500/40 bg-amber-500/15 px-5 text-sm font-semibold text-amber-600 transition-colors hover:bg-amber-500/25 disabled:opacity-50 dark:text-amber-400"
              >
                <Bell size={16} />
                {t('ajustes.notif.activar')}
              </button>
            )}
            {estado.suscrito && (
              <span className="inline-flex items-center gap-1.5 text-[13px] text-emerald-500">
                <Check size={14} />
                {t('ajustes.notif.activadas')}
              </span>
            )}
          </div>
          {estado.permiso === 'denied' && (
            <p className="rounded-xl border border-destructive/30 bg-destructive/10 px-3.5 py-2.5 text-[13px] text-destructive">
              {t('ajustes.notif.permisoDenegado')}
            </p>
          )}
          {estado.error && <p className="text-[13px] text-destructive">{estado.error}</p>}

          {estado.suscrito && prefs && (
            <div className="flex flex-col gap-3 border-t border-app pt-4">
              <p className="text-[13px] font-medium text-muted">{t('ajustes.notif.tiposTitulo')}</p>
              {TIPOS_NOTIF.map((tipo) => (
                <div key={tipo} className="flex items-center justify-between">
                  <span className="text-sm text-app">{t(`ajustes.notif.tipos.${tipo}`)}</span>
                  <button
                    role="switch"
                    aria-checked={prefs[tipo] !== false}
                    aria-label={t(`ajustes.notif.tipos.${tipo}`)}
                    onClick={() => cambiarPref(tipo, prefs[tipo] === false)}
                    className={cn(
                      'relative h-6 w-11 rounded-full transition-colors',
                      prefs[tipo] !== false ? 'bg-amber-500' : 'bg-surface-2',
                    )}
                  >
                    <span
                      className={cn(
                        'absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform',
                        prefs[tipo] !== false ? 'translate-x-[22px]' : 'translate-x-0.5',
                      )}
                    />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ── § Acerca de ──────────────────────────────────────────────────────────────

function AboutSection() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full flex-col items-center justify-center gap-1.5 py-4 text-center text-[13px] text-faint">
      <BrandLogo className="h-12 w-12" />
      <p className="font-medium">{t('ajustes.footer.version', { version: pkg.version })}</p>
      <p>{t('ajustes.footer.local')}</p>
      <p>{t('ajustes.footer.made')}</p>
    </div>
  );
}

// ── Página ───────────────────────────────────────────────────────────────────

export default function Ajustes() {
  const { t } = useTranslation();
  const { state: installState, install } = useInstallPrompt();
  const [, update] = useEnergySettings();
  const [saved, setSaved] = useState(false);
  const [userRole, setUserRole] = useState<string | null>(null);
  const { connectionStatus, liveTick } = useEnergyData();
  const [dataOk, setDataOk] = useState(true);
  const lastTickAt = useRef(0);

  useEffect(() => {
    apiFetch<{ authenticated?: boolean; user?: { role?: string } }>('/api/auth/me')
      .then((data) => {
        if (data.authenticated && data.user) {
          setUserRole(data.user.role ?? null);
        }
      })
      .catch(() => setUserRole(null));
  }, []);

  // Salud del flujo de datos: vivo si el SSE empujó algo en los últimos 15 s.
  useEffect(() => {
    lastTickAt.current = Date.now();
    const id = window.setInterval(() => setDataOk(Date.now() - lastTickAt.current < 15000), 5000);
    return () => window.clearInterval(id);
  }, [liveTick]);

  const saveAll = () => {
    // Aquí se guardarían todos los cambios de todas las secciones
    update({});
    setSaved(true);
    heliosToast(t('ajustes.savedToast'), { tone: 'success' });
    window.setTimeout(() => setSaved(false), 1000);
  };

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      {/* Encabezado */}
      <motion.header initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.4, ease: 'easeOut' }}>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-[-0.01em] text-app">{t('ajustes.title')}</h1>
            <p className="text-sm text-muted">{t('ajustes.subtitle')}</p>
          </div>
          <button
            onClick={saveAll}
            className="bg-brand-gradient inline-flex h-9 min-w-[120px] items-center justify-center gap-2 rounded-full px-6 text-[13px] font-semibold text-white shadow-md transition-transform hover:scale-[1.03] active:scale-95"
          >
            {saved ? (
              <motion.span initial={{ scale: 0.5 }} animate={{ scale: 1 }} transition={{ type: 'spring', stiffness: 500, damping: 20 }} className="flex">
                <Check size={16} strokeWidth={2.6} />
              </motion.span>
            ) : (
              t('common.save')
            )}
          </button>
        </div>
      </motion.header>

      {/* Secciones a todo el ancho (sin mini-nav) */}
      <div className="mx-auto flex w-full max-w-[1200px] flex-col gap-5">
        <Section id="tema" title={t('ajustes.sections.tema')}>
          <ThemeSection />
        </Section>

        {/* Idioma + conexión/datos (unificadas) en la misma horizontal (≥lg); items-start: prohibido estirar la tarjeta corta (hueco interior) */}
        <div className="grid items-start gap-5 lg:grid-cols-2">
          <Section id="idioma" title={t('ajustes.sections.idioma')}>
            <LanguageSection />
          </Section>

          <Section
            id="conexion"
            title={t('ajustes.sections.conexionDatos')}
            badge={<HealthBadge ok={connectionStatus === 'connected' && dataOk} />}
          >
            <div className="flex flex-col gap-4">
              <ConnectionSection />
              <DataSection />
            </div>
          </Section>
        </div>

        <Section id="instalacion" title={t('ajustes.sections.instalacion')}>
          <InstallationSection />
        </Section>

        <Section id="precios" title={t('ajustes.sections.precios')}>
          <PricesSection />
        </Section>

        <Section id="notificaciones" title={t('ajustes.sections.notificaciones')}>
          <NotificationsSection />
        </Section>

        {userRole === 'admin' && (
          <Section id="usuarios" title={t('ajustes.sections.usuarios')}>
            <UsersSection />
          </Section>
        )}

        {/* Mi sesión (baja) se empareja con otra tarjeta baja: Instalar app si el
            navegador la soporta, si no Acerca de. Con PWA visible, Acerca de va a
            ancho completo al final. Nunca una tarjeta muy baja junto a una alta. */}
        {installState !== 'hidden' ? (
          <>
            <div className="grid items-start gap-5 lg:grid-cols-2">
              <Section id="sesion" title={t('ajustes.sections.sesion')}>
                <SessionSection />
              </Section>
              <Section id="app" title={t('ajustes.sections.app')}>
                <InstallSection state={installState} install={install} />
              </Section>
            </div>
            <Section id="acerca" title={t('ajustes.sections.acerca')}>
              <AboutSection />
            </Section>
          </>
        ) : (
          <div className="grid items-start gap-5 lg:grid-cols-2">
            <Section id="sesion" title={t('ajustes.sections.sesion')}>
              <SessionSection />
            </Section>
            <Section id="acerca" title={t('ajustes.sections.acerca')}>
              <AboutSection />
            </Section>
          </div>
        )}

      </div>
    </div>
  );
}
