import { useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';
import {
  BatteryCharging,
  Check,
  House,
  KeyRound,
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
import HeliosToaster from '@/components/HeliosToaster';
import BrandLogo from '@/components/BrandLogo';
import { heliosToast } from '@/lib/toast';
import { LANG_MODE_KEY, resolveNavigatorLanguage } from '@/i18n';
import { fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';

const easeOutQuart = [0.25, 1, 0.5, 1] as [number, number, number, number];

const SECTION_IDS = ['tema', 'idioma', 'conexion', 'instalacion', 'precios', 'datos', 'app'] as const;

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
  innerRef,
}: {
  id: string;
  title: string;
  badge?: ReactNode;
  children: ReactNode;
  innerRef?: (el: HTMLElement | null) => void;
}) {
  return (
    <motion.section
      id={id}
      ref={innerRef}
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

// ── §1 Tema ──────────────────────────────────────────────────────────────────

const THEME_OPTIONS: { value: ThemeMode; labelKey: string; icon: typeof Sun }[] = [
  { value: 'auto', labelKey: 'theme.auto', icon: Sunrise },
  { value: 'light', labelKey: 'theme.light', icon: Sun },
  { value: 'dark', labelKey: 'theme.dark', icon: Moon },
];

const THEME_PREVIEWS = [
  {
    key: 'light',
    labelKey: 'theme.light',
    bg: '#F4F6FA',
    surface: '#FFFFFF',
    text: '#0C1425',
    faint: '#94A3B8',
    border: '#E3E8F0',
    accent: '#F59E0B',
  },
  {
    key: 'dark',
    labelKey: 'theme.dark',
    bg: '#080D1A',
    surface: '#101828',
    text: '#E9EEF7',
    faint: '#5C6B85',
    border: '#1E2B42',
    accent: '#FBBF24',
  },
] as const;

function ThemeSection() {
  const { mode, setMode, effective } = useTheme();
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

      {/* Preview en vivo de ambos temas */}
      <div className="flex flex-wrap gap-3">
        {THEME_PREVIEWS.map((tp, i) => (
          <motion.div
            key={tp.key}
            initial={{ opacity: 0, scale: 0.96 }}
            animate={{ opacity: 1, scale: 1 }}
            transition={{ duration: 0.5, delay: 0.1 + i * 0.1, ease: easeOutQuart }}
            className="w-[180px] rounded-2xl p-2"
            style={{ backgroundColor: tp.bg, border: `1px solid ${tp.border}` }}
            aria-label={t('ajustes.theme.previewAria', { label: t(tp.labelKey) })}
          >
            <div className="flex h-[96px] flex-col justify-between rounded-xl p-3" style={{ backgroundColor: tp.surface, border: `1px solid ${tp.border}` }}>
              <div className="flex items-center justify-between">
                <span
                  className="flex h-6 w-6 items-center justify-center rounded-lg"
                  style={{ backgroundColor: `${tp.accent}1F`, color: tp.accent }}
                >
                  <Sun size={13} strokeWidth={2.4} />
                </span>
                <span className="text-[9px] font-medium uppercase tracking-[0.08em]" style={{ color: tp.faint }}>
                  {t(tp.labelKey)}
                </span>
              </div>
              <div>
                <p className="text-[9px] font-medium uppercase tracking-[0.08em]" style={{ color: tp.faint }}>
                  {t('common.production')}
                </p>
                <p className="font-display text-lg font-semibold leading-tight" style={{ color: tp.text }}>
                  6,12 <span className="text-[0.6em] font-medium" style={{ color: tp.faint }}>kW</span>
                </p>
              </div>
            </div>
          </motion.div>
        ))}
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
      fetch('/api/auth/profile', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ language: value }),
      }).catch(() => {});
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
      const res = await fetch('/api/solar/live', { credentials: 'same-origin' });
      if (!res.ok) throw new Error();
      const data = (await res.json()) as { connected?: boolean; station?: string };
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

function InstallationSection() {
  const [settings, update] = useEnergySettings();
  const { t } = useTranslation();

  const chips = [
    {
      key: 'solis',
      name: 'Solis S5-EH1P5K-L',
      lines: [t('ajustes.install.solisLine1'), t('ajustes.install.solisLine2')],
      color: 'var(--c-solis)',
      icon: Zap,
      extraIcon: BatteryCharging,
      from: -24,
    },
    {
      key: 'fox',
      name: 'Fox H1-3.0-E',
      lines: [t('ajustes.install.foxLine1')],
      color: 'var(--c-fox)',
      icon: Zap,
      extraIcon: null,
      from: 24,
    },
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 md:grid-cols-2">
        {chips.map((c, i) => (
          <motion.div
            key={c.key}
            initial={{ opacity: 0, x: c.from }}
            whileInView={{ opacity: 1, x: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.5, delay: i * 0.12, ease: easeOutQuart }}
            className="group rounded-xl border-2 bg-surface p-4 transition-colors"
            style={{ borderColor: `color-mix(in srgb, ${c.color} 35%, transparent)` }}
          >
            <div className="mb-2 flex items-center gap-2">
              <span
                className="flex h-8 w-8 items-center justify-center rounded-lg transition-transform group-hover:scale-110"
                style={{ backgroundColor: `color-mix(in srgb, ${c.color} 12%, transparent)`, color: c.color }}
              >
                <c.icon size={16} strokeWidth={2.2} />
              </span>
              <p className="font-display text-[15px] font-semibold text-app">{c.name}</p>
            </div>
            {c.lines.map((l) => (
              <p key={l} className="text-[13px] leading-relaxed text-muted">
                {l}
              </p>
            ))}
          </motion.div>
        ))}
      </div>

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

function PricesSection() {
  const [settings] = useEnergySettings();
  const { t } = useTranslation();
  const [priceImport, setPriceImport] = useState(() => toEsDecimal(settings.priceImport));
  const [priceExport, setPriceExport] = useState(() => toEsDecimal(settings.priceExport));
  const [co2, setCo2] = useState(() => toEsDecimal(settings.co2Factor));

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

// ── §7 Usuarios (solo admin) ────────────────────────────────────────────────

function UsersSection() {
  const { t } = useTranslation();
  const [users, setUsers] = useState<any[]>([]);
  const [meId, setMeId] = useState<string | null>(null);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [language, setLanguage] = useState('es');
  const [role, setRole] = useState<'user' | 'admin'>('user');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pwdFor, setPwdFor] = useState<string | null>(null);
  const [newPwd, setNewPwd] = useState('');

  const languages = [
    { code: 'es', name: 'Español', flag: '🇪🇸' },
    { code: 'en', name: 'English', flag: '🇬🇧' },
    { code: 'zh-CN', name: '简体中文', flag: '🇨🇳' },
  ];

  const reload = () => {
    fetch('/api/auth/users', { credentials: 'same-origin' })
      .then((res) => res.json())
      .then((data) => setUsers(data.users || []))
      .catch(() => setUsers([]));
  };

  useEffect(() => {
    reload();
    fetch('/api/auth/me', { credentials: 'same-origin' })
      .then((res) => res.json())
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
      const res = await fetch('/api/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ username, password, language, role }),
      });
      const body = (await res.json()) as { error?: string; ok?: boolean; user?: unknown };
      if (!res.ok) {
        setError(body?.error ?? t('common.error'));
        return;
      }
      heliosToast(t('admin.users.createdToast', { username }), { tone: 'success' });
      setUsername('');
      setPassword('');
      setLanguage('es');
      setRole('user');
      reload();
    } catch {
      setError(t('common.error'));
    } finally {
      setBusy(false);
    }
  };

  const changePassword = async (userId: string) => {
    if (newPwd.length < 6) return;
    const res = await fetch(`/api/auth/users/${userId}/password`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ password: newPwd }),
    });
    if (res.ok) {
      heliosToast(t('admin.users.passwordChanged'), { tone: 'success' });
      setPwdFor(null);
      setNewPwd('');
    } else {
      heliosToast(t('common.error'), { tone: 'warning' });
    }
  };

  const changeLanguage = async (userId: string, lang: string) => {
    const res = await fetch(`/api/auth/users/${userId}/language`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ language: lang }),
    });
    if (res.ok) {
      heliosToast(t('admin.users.languageChanged'), { tone: 'success' });
      setUsers((us) => us.map((u) => (u.id === userId ? { ...u, language: lang } : u)));
    } else {
      heliosToast(t('common.error'), { tone: 'warning' });
    }
  };

  const changeRole = async (userId: string, newRole: string) => {
    const res = await fetch(`/api/auth/users/${userId}/role`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ role: newRole }),
    });
    if (res.ok) {
      heliosToast(t('admin.users.roleChanged'), { tone: 'success' });
      setUsers((us) => us.map((u) => (u.id === userId ? { ...u, role: newRole } : u)));
    } else {
      heliosToast(t('common.error'), { tone: 'warning' });
    }
  };

  const deleteUser = async (userId: string, name: string) => {
    if (!window.confirm(t('admin.users.deleteConfirm', { username: name }))) return;
    const res = await fetch(`/api/auth/users/${userId}`, { method: 'DELETE', credentials: 'same-origin' });
    if (res.ok) {
      heliosToast(t('admin.users.deleted'), { tone: 'success' });
      setUsers((us) => us.filter((u) => u.id !== userId));
    } else {
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
                  {/* Eliminar (no sobre uno mismo) */}
                  {user.id !== meId && (
                    <button
                      onClick={() => deleteUser(user.id, user.username)}
                      title={t('admin.users.delete')}
                      aria-label={t('admin.users.delete')}
                      className="flex h-8 w-8 items-center justify-center rounded-lg border border-destructive/30 text-destructive transition-colors hover:bg-destructive/10"
                    >
                      <Trash2 size={14} />
                    </button>
                  )}
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

      {/* Formulario crear usuario */}
      <form onSubmit={submit} className="flex flex-col gap-4 border-t border-app pt-4">
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

        <button
          type="submit"
          disabled={busy || !username || !password}
          className="bg-brand-gradient inline-flex h-10 items-center justify-center gap-2 rounded-full text-sm font-semibold text-white shadow-md transition-transform hover:scale-[1.02] active:scale-95 disabled:opacity-60 disabled:hover:scale-100"
        >
          <UserPlus size={16} />
          {busy ? t('admin.users.creating') : t('admin.users.create')}
        </button>
      </form>
    </div>
  );
}

// ── §8 Instalar como app (PWA) ───────────────────────────────────────────────

function InstallSection() {
  const [deferredPrompt, setDeferredPrompt] = useState<BeforeInstallPromptEvent | null>(null);
  const { t } = useTranslation();
  const { isDark } = useTheme();

  useEffect(() => {
    const handler = (e: Event) => {
      e.preventDefault();
      setDeferredPrompt(e as BeforeInstallPromptEvent);
    };
    window.addEventListener('beforeinstallprompt', handler);
    return () => window.removeEventListener('beforeinstallprompt', handler);
  }, []);

  const install = async () => {
    if (!deferredPrompt) return;
    await deferredPrompt.prompt();
    setDeferredPrompt(null);
  };

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
        <ol className="mt-3 flex flex-col gap-1.5">
          {steps.map((s, i) => (
            <motion.li
              key={s}
              initial={{ opacity: 0, y: 12 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.4, delay: 0.15 + i * 0.1, ease: 'easeOut' }}
              className="flex items-baseline gap-2 text-sm text-muted"
            >
              <span className="flex h-5 w-5 shrink-0 translate-y-0.5 items-center justify-center rounded-full bg-surface-2 text-[11px] font-semibold text-muted">
                {i + 1}
              </span>
              {s}
            </motion.li>
          ))}
        </ol>
        <div className="mt-6 flex justify-center">
          {deferredPrompt ? (
            <button
              onClick={install}
              className="bg-brand-gradient inline-flex h-14 items-center gap-3 rounded-full px-8 text-[16px] font-semibold text-white shadow-lg transition-transform hover:scale-[1.03] active:scale-95"
            >
              <Smartphone size={22} />
              {t('ajustes.pwa.install')}
            </button>
          ) : (
            <p className="inline-flex items-center gap-2 rounded-full border border-app bg-surface-2 px-6 py-3 text-sm text-muted">
              <Smartphone size={18} className="text-faint" />
              {t('ajustes.pwa.unavailable')}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

// ── Página ───────────────────────────────────────────────────────────────────

export default function Ajustes() {
  const [activeSection, setActiveSection] = useState<string>('tema');
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({});
  const { t } = useTranslation();
  const [, update] = useEnergySettings();
  const [saved, setSaved] = useState(false);
  const [userRole, setUserRole] = useState<string | null>(null);

  useEffect(() => {
    fetch('/api/auth/me', { credentials: 'same-origin' })
      .then((res) => res.json())
      .then((data) => {
        if (data.authenticated && data.user) {
          setUserRole(data.user.role);
        }
      })
      .catch(() => setUserRole(null));
  }, []);

  // Scroll-spy: resalta la sección visible en la mini-nav (≥xl).
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) setActiveSection(entry.target.id);
        }
      },
      { rootMargin: '-40% 0px -55% 0px', threshold: 0 },
    );
    for (const s of SECTION_IDS) {
      const el = sectionRefs.current[s];
      if (el) observer.observe(el);
    }
    return () => observer.disconnect();
  }, []);

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

      <div className="xl:flex xl:items-start xl:justify-center xl:gap-10">
        {/* Mini-nav anclada (solo ≥xl) */}
        <aside className="hidden w-[150px] shrink-0 xl:block">
          <nav aria-label={t('ajustes.navAria')} className="sticky top-24 flex flex-col gap-0.5">
            {SECTION_IDS.map((id) => {
              const active = activeSection === id;
              return (
                <button
                  key={id}
                  onClick={() => sectionRefs.current[id]?.scrollIntoView({ behavior: 'smooth', block: 'start' })}
                  aria-current={active}
                  className={cn(
                    'relative rounded-lg px-3 py-2 text-left text-[13px] font-medium transition-colors',
                    active ? 'bg-surface-2 text-amber-600 dark:text-amber-400' : 'text-faint hover:bg-surface-2/50 hover:text-muted',
                  )}
                >
                  {active && (
                    <motion.span layoutId="ajustes-nav-bar" className="bg-brand-gradient absolute left-0 top-2 h-5 w-[3px] rounded-full" />
                  )}
                  {t(`ajustes.sections.${id}Short`)}
                </button>
              );
            })}
          </nav>
        </aside>

        {/* Columna de secciones */}
        <div className="mx-auto flex w-full max-w-[860px] flex-col gap-5 xl:mx-0">
          <Section id="tema" title={t('ajustes.sections.tema')} innerRef={(el) => (sectionRefs.current.tema = el)}>
            <ThemeSection />
          </Section>

          <Section id="idioma" title={t('ajustes.sections.idioma')} innerRef={(el) => (sectionRefs.current.idioma = el)}>
            <LanguageSection />
          </Section>

          <Section id="conexion" title={t('ajustes.sections.conexion')} innerRef={(el) => (sectionRefs.current.conexion = el)}>
            <ConnectionSection />
          </Section>

          <Section id="instalacion" title={t('ajustes.sections.instalacion')} innerRef={(el) => (sectionRefs.current.instalacion = el)}>
            <InstallationSection />
          </Section>

          <Section id="precios" title={t('ajustes.sections.precios')} innerRef={(el) => (sectionRefs.current.precios = el)}>
            <PricesSection />
          </Section>

          <Section id="datos" title={t('ajustes.sections.datos')} innerRef={(el) => (sectionRefs.current.datos = el)}>
            <DataSection />
          </Section>

          {userRole === 'admin' && (
            <Section id="usuarios" title={t('ajustes.sections.usuarios')} innerRef={(el) => (sectionRefs.current.usuarios = el)}>
              <UsersSection />
            </Section>
          )}

          <Section id="app" title={t('ajustes.sections.app')} innerRef={(el) => (sectionRefs.current.app = el)}>
            <InstallSection />
          </Section>

          {/* §8 Cerrar sesión */}
          <motion.div
            initial={{ opacity: 0, y: 12 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.4, delay: 0.1 }}
            className="flex justify-center py-4"
          >
            <button
              onClick={async () => {
                await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' });
                window.location.href = '/login';
              }}
              className="inline-flex h-12 items-center gap-2 rounded-full border border-destructive/30 bg-destructive/10 px-8 text-[15px] font-semibold text-destructive transition-all hover:bg-destructive/20 hover:scale-[1.02] active:scale-95"
            >
              <LogOut size={20} />
              {t('common.logout')}
            </button>
          </motion.div>

          {/* §9 Acerca de */}
          <motion.div
            initial={{ opacity: 0 }}
            whileInView={{ opacity: 1 }}
            viewport={{ once: true }}
            transition={{ duration: 0.6 }}
            className="flex flex-col items-center gap-1.5 py-6 text-center text-[13px] text-faint"
          >
            <BrandLogo className="h-7 w-7" />
            <p className="font-medium">{t('ajustes.footer.version')}</p>
            <p>{t('ajustes.footer.local')}</p>
            <p>{t('ajustes.footer.made')}</p>
          </motion.div>
        </div>
      </div>

      <HeliosToaster />
    </div>
  );
}
