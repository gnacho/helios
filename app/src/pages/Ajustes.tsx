import { useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';
import {
  BatteryCharging,
  Bell,
  Check,
  ChevronDown,
  FileText,
  Github,
  Heart,
  HeartPulse,
  House,
  KeyRound,
  LayoutGrid,
  LogOut,
  MapPin,
  Moon,
  Pencil,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Sun,
  Sunrise,
  Trash2,
  User,
  UserPlus,
  X,
  Zap,
} from 'lucide-react';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { useTheme } from '@/theme/ThemeProvider';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { THEME_BG, THEME_SURFACE, THEME_BAR, ACCENTS } from '@/lib/colors';
import { useEnergySettings } from '@/hooks/useEnergySettings';
import { usePush } from '@/hooks/usePush';
import BrandLogo from '@/components/BrandLogo';
import { heliosToast } from '@/lib/toast';
import { LANG_MODE_KEY, resolveNavigatorLanguage, numLocale } from '@/i18n';
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
  className,
  children,
}: {
  id: string;
  title: string;
  badge?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <motion.section
      id={id}
      initial={{ opacity: 0, y: 24 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.15 }}
      transition={{ duration: 0.5, ease: easeOutQuart }}
      className={cn('helios-card scroll-mt-20 p-5 shadow-card dark:shadow-card-dark lg:scroll-mt-6', className)}
    >
      <div className="mb-4 flex items-center gap-2.5">
        <h2 className="text-[15px] font-semibold text-app">{title}</h2>
        {badge}
      </div>
      {children}
    </motion.section>
  );
}

/** Sección de auditoría (AdminBar): consume GET /api/auth/audit (solo admin). */
interface AuditEntry {
  id: number;
  ts: number;
  actor: string;
  action: string;
  detail?: string | null;
}

function AuditSection() {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setBusy(true);
    apiFetch<{ entries?: AuditEntry[] }>('/api/auth/audit?limit=50')
      .then((d) => setEntries(d.entries ?? []))
      .catch(() => setEntries([]))
      .finally(() => setBusy(false));
  }, []);

  if (busy) {
    return <p className="text-[13px] text-muted">{t('ajustes.about.checking')}</p>;
  }

  if (entries.length === 0) {
    return <p className="text-[13px] text-muted">{t('audit.empty')}</p>;
  }

  const fmtTs = (ms: number) => {
    try {
      return new Intl.DateTimeFormat(undefined, {
        day: '2-digit',
        month: 'short',
        hour: '2-digit',
        minute: '2-digit',
      }).format(new Date(ms));
    } catch {
      return String(ms);
    }
  };

  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('audit.when')}</TableHead>
            <TableHead>{t('audit.actor')}</TableHead>
            <TableHead>{t('audit.action')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map((e) => (
            <TableRow key={e.id}>
              <TableCell className="whitespace-nowrap text-[13px] text-muted">{fmtTs(e.ts)}</TableCell>
              <TableCell className="text-[13px]">{e.actor}</TableCell>
              <TableCell className="text-[13px] text-muted">{e.action}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

/** Zona de administración (patrón AdminBar del skill): tarjeta horizontal con
 *  borde de acento, título "Administración", secciones en la barra y paneles
 *  desplegables debajo. Orden canónico: Actualizaciones → Usuarios → Auditoría
 *  (Respaldos y Modo demo no existen en Helios). */
function AdminZone() {
  const { t } = useTranslation();
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [checking, setChecking] = useState(false);
  const [updateStatus, setUpdateStatus] = useState<'idle' | 'uptodate' | 'available' | 'error'>('idle');
  const [latestVersion, setLatestVersion] = useState('');
  const [applying, setApplying] = useState(false);

  const applyUpdate = async () => {
    if (applying) return;
    setApplying(true);
    try {
      await apiPost<{ ok: boolean }>('/api/update/apply');
      // El servidor se reinicia; la app se recarga con el build nuevo.
      setTimeout(() => window.location.reload(), 2500);
    } catch {
      setApplying(false);
      setUpdateStatus('error');
    }
  };

  const checkUpdates = async () => {
    setChecking(true);
    setUpdateStatus('idle');
    try {
      const repo = REPO_URL.match(/github\.com\/([^/]+\/[^/.]+)/)?.[1];
      if (!repo) throw new Error('no repo');
      const res = await fetch(`https://api.github.com/repos/${repo}/releases/latest`, {
        headers: { Accept: 'application/vnd.github+json' },
      });
      let version = '';
      if (res.ok) {
        const data = await res.json();
        version = data.tag_name || '';
      } else if (res.status === 404) {
        const tagRes = await fetch(`https://api.github.com/repos/${repo}/tags?per_page=1`, {
          headers: { Accept: 'application/vnd.github+json' },
        });
        if (tagRes.ok) {
          const tags = await tagRes.json();
          version = tags[0]?.name || '';
        }
      }
      if (!version || compareSemver(version, pkg.version) <= 0) {
        setUpdateStatus('uptodate');
      } else {
        setLatestVersion(version);
        setUpdateStatus('available');
      }
    } catch {
      setUpdateStatus('error');
    } finally {
      setChecking(false);
    }
  };

  const toggle = (id: string) => setOpen((prev) => ({ ...prev, [id]: !prev[id] }));

  const btnCls = (active: boolean) =>
    cn(
      'inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-[13px] font-medium transition-colors shrink-0',
      active
        ? 'border-brand bg-brand/10 text-brand'
        : 'border-app bg-surface text-muted hover:bg-surface-2 hover:text-app',
    );

  return (
    <motion.section
      initial={{ opacity: 0, y: 24 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.15 }}
      transition={{ duration: 0.5, ease: easeOutQuart }}
      className="helios-card scroll-mt-20 border-l-4 border-l-brand bg-brand/[0.03] p-5 shadow-card dark:shadow-card-dark"
    >
      {/* Fila horizontal de la barra */}
      <div className="flex flex-wrap items-start gap-3">
        <div className="flex h-9 items-center gap-2 shrink-0">
          <ShieldCheck size={18} strokeWidth={1.75} className="text-brand" />
          <h2 className="font-display text-[15px] font-semibold text-app">{t('ajustes.sections.administracion')}</h2>
        </div>
        <div className="hidden h-6 w-px bg-app sm:block" />

        {/* 1. Comprobar actualizaciones (widget inline) */}
        <div className="flex flex-col gap-1">
          <button
            type="button"
            onClick={checkUpdates}
            disabled={checking}
            className="inline-flex h-9 items-center gap-2 rounded-lg border border-app bg-surface px-3 text-[13px] font-medium text-muted transition-colors hover:bg-surface-2 hover:text-app disabled:opacity-60"
          >
            {checking && <RefreshCw size={13} className="animate-spin" />}
            <span className="hidden sm:inline">
              {checking ? t('ajustes.about.checking') : t('ajustes.about.checkUpdates')}
            </span>
          </button>
          {updateStatus === 'uptodate' && (
            <span className="text-[10px] font-medium text-emerald-500">
              {t('ajustes.about.upToDate', { version: pkg.version })}
            </span>
          )}
          {updateStatus === 'available' && (
            <>
              <a href={`${REPO_URL}/releases`} target="_blank" rel="noreferrer" className="text-[10px] font-medium text-brand">
                {t('ajustes.about.updateAvailable', { version: latestVersion })}
              </a>
              <button
                type="button"
                onClick={() => void applyUpdate()}
                disabled={applying}
                className="inline-flex h-7 items-center gap-1.5 rounded-lg border border-brand bg-brand/10 px-2.5 text-[11px] font-semibold text-brand transition-colors hover:bg-brand/20 disabled:opacity-60"
              >
                {applying ? t('ajustes.about.applying') : t('ajustes.about.updateNow')}
              </button>
            </>
          )}
          {updateStatus === 'error' && (
            <span role="alert" className="text-[10px] font-medium text-destructive">
              {t('ajustes.about.updateError')}
            </span>
          )}
        </div>

        {/* 2. Usuarios (desplegable) */}
        <button
          type="button"
          aria-expanded={!!open.users}
          onClick={() => toggle('users')}
          className={btnCls(!!open.users)}
        >
          <UserPlus size={16} strokeWidth={1.75} />
          <span className="hidden sm:inline">{t('ajustes.sections.usuarios')}</span>
          <ChevronDown size={14} className={cn('transition-transform', open.users && 'rotate-180')} />
        </button>

        {/* 3. Auditoría (desplegable) */}
        <button
          type="button"
          aria-expanded={!!open.audit}
          onClick={() => toggle('audit')}
          className={btnCls(!!open.audit)}
        >
          <FileText size={16} strokeWidth={1.75} />
          <span className="hidden sm:inline">{t('audit.title')}</span>
          <ChevronDown size={14} className={cn('transition-transform', open.audit && 'rotate-180')} />
        </button>
      </div>

      {/* Paneles desplegados */}
      {open.users && (
        <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="mt-4 border-t border-app pt-4">
          <UsersSection />
        </motion.div>
      )}
      {open.audit && (
        <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="mt-4 border-t border-app pt-4">
          <AuditSection />
        </motion.div>
      )}
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

/** Mitad del preview 'split': un lado del tema con sus tokens reales. */
function PreviewBlock({ useLight }: { useLight: boolean }) {
  const bgC = useLight ? THEME_BG.light : THEME_BG.dark;
  const surfaceC = useLight ? THEME_SURFACE.light : THEME_SURFACE.dark;
  const barC = useLight ? THEME_BAR.light : THEME_BAR.dark;
  return (
    <div className="flex w-1/2 flex-col p-1.5" style={{ backgroundColor: bgC }}>
      <div className="mb-1 h-1.5 w-full rounded" style={{ backgroundColor: barC }} />
      <div className="flex flex-1 gap-1">
        <div className="w-1/4 rounded" style={{ backgroundColor: barC }} />
        <div className="flex flex-1 flex-col gap-1">
          <div className="h-2 w-3/4 rounded bg-brand/60" />
          <div className="h-4 flex-1 rounded" style={{ backgroundColor: surfaceC }} />
        </div>
      </div>
    </div>
  );
}

/** Mini-preview de tema: dibuja una mini-UI con divs usando los tokens reales. */
function ThemePreview({ variant }: { variant: 'dark' | 'light' | 'split' }) {
  const { bg, surface, bar } = {
    dark: { bg: THEME_BG.dark, surface: THEME_SURFACE.dark, bar: THEME_BAR.dark },
    light: { bg: THEME_BG.light, surface: THEME_SURFACE.light, bar: THEME_BAR.light },
  }[variant === 'split' ? 'dark' : variant];
  const accent = 'bg-brand';

  if (variant === 'split') {
    return (
      <div className="flex h-[72px] w-full overflow-hidden rounded-lg border border-app">
        <PreviewBlock useLight={false} />
        <PreviewBlock useLight />
      </div>
    );
  }

  return (
    <div className="flex h-[72px] w-full flex-col rounded-lg border border-app p-1.5" style={{ backgroundColor: bg }}>
      <div className="mb-1 h-1.5 w-full rounded" style={{ backgroundColor: bar }} />
      <div className="flex flex-1 gap-1">
        <div className="w-1/4 rounded" style={{ backgroundColor: bar }} />
        <div className="flex flex-1 flex-col gap-1">
          <div className={cn('h-2 w-3/4 rounded', accent)} />
          <div className="h-4 flex-1 rounded" style={{ backgroundColor: surface }} />
        </div>
      </div>
    </div>
  );
}

/** Acenos disponibles (paleta en lib/colors.ts). */

function ThemeSection() {
  const { mode, setMode, density, setDensity, reduceMotion, setReduceMotion, accent, setAccent } = useTheme();
  const { t } = useTranslation();

  const themeOptions = [
    { value: 'dark' as const, label: t('theme.dark'), icon: Moon },
    { value: 'light' as const, label: t('theme.light'), icon: Sun },
    { value: 'auto' as const, label: t('theme.auto'), icon: Sunrise },
  ];

  return (
    <div className="flex flex-col gap-4">
      {/* Tema: radiogroup con mini-previews */}
      <div role="radiogroup" aria-label={t('theme.label')} className="grid grid-cols-3 gap-2">
        {themeOptions.map(({ value, label, icon: Icon }) => {
          const active = mode === value;
          return (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={active}
              onClick={() => setMode(value)}
              className={cn(
                'group relative flex flex-col gap-2 rounded-xl border-2 p-2 transition-all',
                active ? 'border-brand bg-brand/5' : 'border-app hover:border-brand/30',
              )}
            >
              <ThemePreview variant={value === 'auto' ? 'split' : value} />
              <div className="flex items-center justify-center gap-1.5">
                <Icon size={14} className={active ? 'text-brand' : 'text-faint'} />
                <span className={cn('text-xs font-medium', active ? 'text-brand' : 'text-muted')}>{label}</span>
              </div>
              {active && (
                <motion.span
                  initial={{ scale: 0 }}
                  animate={{ scale: 1 }}
                  transition={{ type: 'spring', stiffness: 500, damping: 30 }}
                  className="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full bg-brand text-white"
                >
                  <Check size={12} strokeWidth={3} />
                </motion.span>
              )}
            </button>
          );
        })}
      </div>

      {/* Acento */}
      <div>
        <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.accent.title')}</p>
        <div role="radiogroup" aria-label={t('ajustes.accent.title')} className="flex items-center gap-2.5">
          {ACCENTS.map((acc) => {
            const active = accent === acc.rgb;
            return (
              <button
                key={acc.id}
                type="button"
                role="radio"
                aria-checked={active}
                aria-label={t(`ajustes.accent.${acc.id}`)}
                title={t(`ajustes.accent.${acc.id}`)}
                onClick={() => setAccent(acc.rgb)}
                className={cn(
                  'relative flex h-8 w-8 items-center justify-center rounded-full transition-transform hover:scale-110',
                  active && 'ring-2 ring-brand ring-offset-2 ring-offset-[var(--surface)]',
                )}
                style={{ backgroundColor: acc.hex }}
              >
                {active && <Check size={14} strokeWidth={3} className="text-white" />}
              </button>
            );
          })}
        </div>
      </div>

      {/* Densidad */}
      <div>
        <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.density.title')}</p>
        <div role="radiogroup" aria-label={t('ajustes.density.title')} className="flex rounded-xl border border-app p-0.5">
          {(['comfortable', 'compact'] as const).map((d) => (
            <button
              key={d}
              type="button"
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
        <Switch checked={reduceMotion} onCheckedChange={setReduceMotion} />
      </div>
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
      <div className="grid gap-3 md:grid-cols-3">
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
            <div className="mb-3 flex h-10 items-center gap-2">
              <span
                className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-transform group-hover:scale-110"
                style={{ backgroundColor: `color-mix(in srgb, ${inv.color} 12%, transparent)`, color: inv.color }}
              >
                <Zap size={16} strokeWidth={2.2} />
              </span>
              <p className="min-w-0 truncate font-display text-[15px] font-semibold text-app" title={inv.name}>
                {inv.name}
              </p>
            </div>

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

        {/* Capacidad teórica TOTAL como 3ª tarjeta */}
        <motion.div
          initial={{ opacity: 0, x: 24 }}
          whileInView={{ opacity: 1, x: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5, delay: 0.24, ease: easeOutQuart }}
          className="group rounded-xl border-2 border-brand/35 bg-surface p-4"
        >
          <div className="mb-3 flex h-10 items-center gap-2">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-brand/12 text-brand transition-transform group-hover:scale-110">
              <Sun size={16} strokeWidth={2.2} />
            </span>
            <p className="min-w-0 truncate font-display text-[15px] font-semibold text-app" title={t('ajustes.install.totalLabel')}>
              {t('ajustes.install.totalLabel')}
            </p>
          </div>

          <div>
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.install.theoretical')}</p>
            <p className="font-display text-3xl font-bold leading-tight text-app">
              {fmtKwp(TOTAL_KWP)} <span className="text-base font-medium text-faint">kWp</span>
            </p>

            <div className="mt-3 flex flex-col gap-2">
              <p className="flex items-center gap-2 text-[13px] text-muted">
                <Zap size={15} className="shrink-0 text-faint" />
                {INVERTERS.length} {t('ajustes.install.inverters')}
              </p>
              <p className="flex items-center gap-2 text-[13px] text-muted">
                <LayoutGrid size={15} className="shrink-0 text-faint" />
                {INVERTERS.reduce((a, inv) => a + inv.panels, 0)} {t('ajustes.install.panelsShort')}
              </p>
            </div>
          </div>
        </motion.div>
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

// ── §7c Mi perfil (nombre visible, email, idioma, avatar) ────────────────────

const TIPOS_NOTIF_USUARIO = [
  'inversor_offline',
  'inversor_ok',
  'corte_red',
  'corte_red_ok',
  'bateria_baja',
  'resumen_diario',
] as const;

function ProfileSection() {
  const { t, i18n } = useTranslation();
  const { estado } = usePush();
  const [user, setUser] = useState<{ id: string; username: string; display_name?: string | null; email?: string | null; language: string } | null>(null);
  const [editing, setEditing] = useState(false);
  const [displayName, setDisplayName] = useState('');
  const [showPwd, setShowPwd] = useState(false);
  const [pwdCurrent, setPwdCurrent] = useState('');
  const [pwdNew, setPwdNew] = useState('');
  const [pwdConfirm, setPwdConfirm] = useState('');
  const [pwdBusy, setPwdBusy] = useState(false);
  const [pwdError, setPwdError] = useState<string | null>(null);
  const [showNotifs, setShowNotifs] = useState(false);
  const [prefs, setPrefs] = useState<Record<string, boolean> | null>(null);

  const languages = [
    { code: 'auto', name: t('ajustes.language.auto'), flag: '🌐' },
    { code: 'es', name: 'Español', flag: '🇪🇸' },
    { code: 'en', name: 'English', flag: '🇬🇧' },
    { code: 'zh-CN', name: '简体中文', flag: '🇨🇳' },
  ];

  useEffect(() => {
    // Obtener username rápidamente de /api/auth/me
    apiFetch<{ authenticated?: boolean; user?: { username?: string } }>('/api/auth/me')
      .then((d) => {
        if (d.authenticated && d.user?.username) {
          setDisplayName(d.user.username);
        }
      })
      .catch(() => {});
    
    // Obtener perfil completo de /api/auth/profile
    apiFetch<{ authenticated?: boolean; user?: { id: string; username: string; display_name?: string | null; email?: string | null; language: string } }>(
      '/api/auth/profile',
    )
      .then((d) => {
        if (d.authenticated && d.user) {
          setUser(d.user);
          setDisplayName(d.user.display_name || d.user.username);
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    apiFetch<{ prefs: Record<string, boolean> }>('/api/push/preferences')
      .then((r) => setPrefs(r.prefs))
      .catch(() => setPrefs(null));
  }, []);

  const cambiarPref = (tipo: string, enabled: boolean) => {
    setPrefs((p) => (p ? { ...p, [tipo]: enabled } : p));
    apiPut('/api/push/preferences', { tipo, enabled }).catch(() => {
      setPrefs((p) => (p ? { ...p, [tipo]: !enabled } : p));
    });
  };

  const saveName = async () => {
    try {
      await apiPut('/api/auth/profile', { display_name: displayName.trim() || null });
      heliosToast(t('ajustes.profile.saved'), { tone: 'success' });
      setEditing(false);
    } catch {
      heliosToast(t('common.error'), { tone: 'warning' });
    }
  };

  const handleLang = (value: string) => {
    if (value === 'auto') {
      localStorage.setItem(LANG_MODE_KEY, 'auto');
      i18n.changeLanguage(resolveNavigatorLanguage());
    } else {
      localStorage.setItem(LANG_MODE_KEY, 'manual');
      i18n.changeLanguage(value);
      apiPut('/api/auth/profile', { language: value }).catch(() => {});
    }
  };

  const doLogout = async () => {
    try {
      await apiPost('/api/auth/logout');
    } finally {
      window.dispatchEvent(new Event('helios-unauthorized'));
    }
  };

  const submitPwd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (pwdBusy || pwdNew !== pwdConfirm || pwdNew.length < 6) return;
    setPwdBusy(true);
    setPwdError(null);
    try {
      await apiPut('/api/auth/password', { current: pwdCurrent, password: pwdNew });
      heliosToast(t('ajustes.security.changed'), { tone: 'success' });
      setShowPwd(false);
      setPwdCurrent('');
      setPwdNew('');
      setPwdConfirm('');
    } catch (err) {
      setPwdError(err instanceof ApiError ? err.message : t('common.error'));
    } finally {
      setPwdBusy(false);
    }
  };

  const isAuto = localStorage.getItem(LANG_MODE_KEY) === 'auto';
  const langValue = isAuto ? 'auto' : (user?.language ?? 'es');

  return (
    <div className="flex flex-col gap-3">
      {/* Línea principal compacta */}
      <div className="flex items-center gap-3">
        {/* Avatar con icono User */}
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-brand/12 text-brand">
          <User size={20} />
        </div>
        
        {/* Nombre + idioma + acciones izquierda */}
        <div className="min-w-0 flex-1 flex items-center gap-3">
          {editing ? (
            <div className="flex gap-2 flex-1">
              <Input
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder={user?.username}
                className="h-8 flex-1"
              />
              <button
                type="button"
                onClick={saveName}
                className="inline-flex h-8 items-center rounded bg-brand px-2.5 text-xs font-semibold text-white"
              >
                <Check size={14} />
              </button>
              <button
                type="button"
                onClick={() => { setEditing(false); setDisplayName(user?.display_name || user?.username || ''); }}
                className="inline-flex h-8 items-center rounded border border-app px-2.5 text-xs text-muted"
              >
                <X size={14} />
              </button>
            </div>
          ) : (
            <>
              <button
                type="button"
                onClick={() => setEditing(true)}
                className="flex items-center gap-2 min-w-0 group"
              >
                <p className="font-display text-base font-semibold text-app truncate">{displayName || user?.username || ''}</p>
                <Pencil size={13} className="text-faint opacity-0 group-hover:opacity-100 transition-opacity" />
              </button>
              <Select value={langValue} onValueChange={handleLang}>
                <SelectTrigger className="h-8 w-[120px] text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {languages.map((lang) => (
                    <SelectItem key={lang.code} value={lang.code}>
                      <span className="flex items-center gap-2"><span>{lang.flag}</span><span>{lang.name}</span></span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <button
                type="button"
                onClick={() => setShowPwd((v) => !v)}
                aria-expanded={showPwd}
                className="flex h-9 items-center gap-1.5 rounded-lg px-2.5 text-faint transition-colors hover:bg-surface-2 hover:text-app sm:px-3"
                title={t('ajustes.security.change')}
              >
                <KeyRound size={16} />
                <span className="hidden text-sm font-medium sm:inline">Contraseña</span>
              </button>
              <button
                type="button"
                onClick={() => setShowNotifs((v) => !v)}
                aria-expanded={showNotifs}
                className={cn(
                  'flex h-9 items-center gap-1.5 rounded-lg px-2.5 transition-colors sm:px-3',
                  showNotifs ? 'bg-brand/15 text-brand' : 'text-faint hover:bg-surface-2 hover:text-app',
                )}
                title={t('ajustes.sections.notificaciones')}
              >
                <Bell size={16} />
                <span className="hidden text-sm font-medium sm:inline">{t('ajustes.sections.notificaciones')}</span>
              </button>
            </>
          )}
        </div>

        {/* Cerrar sesión a la derecha */}
        <button
          type="button"
          onClick={doLogout}
          className="flex h-9 items-center gap-1.5 rounded-lg border border-destructive/30 bg-destructive/10 px-2.5 text-destructive/80 transition-colors hover:bg-destructive/20 hover:text-destructive sm:px-3"
          title={t('common.logout')}
        >
          <LogOut size={16} />
          <span className="text-sm font-medium">{t('common.logout')}</span>
        </button>
      </div>

      {/* Formulario de contraseña (expandible) */}
      {showPwd && (
        <form onSubmit={submitPwd} className="flex flex-col gap-2 border-t border-app pt-3">
          <Input
            type="password"
            autoComplete="current-password"
            placeholder={t('ajustes.security.current')}
            value={pwdCurrent}
            onChange={(e) => setPwdCurrent(e.target.value)}
            className="h-8 text-sm"
          />
          <div className="grid gap-2 sm:grid-cols-2">
            <Input
              type="password"
              autoComplete="new-password"
              placeholder={t('ajustes.security.new')}
              value={pwdNew}
              onChange={(e) => setPwdNew(e.target.value)}
              className="h-8 text-sm"
            />
            <Input
              type="password"
              autoComplete="new-password"
              placeholder={t('ajustes.security.confirm')}
              value={pwdConfirm}
              onChange={(e) => setPwdConfirm(e.target.value)}
              className="h-8 text-sm"
            />
          </div>
          {pwdNew.length > 0 && pwdNew !== pwdConfirm && (
            <p className="text-xs text-destructive">{t('ajustes.security.mismatch')}</p>
          )}
          {pwdError && <p className="text-xs text-destructive">{pwdError}</p>}
          <button
            type="submit"
            disabled={pwdBusy || !pwdCurrent || pwdNew.length < 6 || pwdNew !== pwdConfirm}
            className="bg-brand-gradient inline-flex h-8 self-start items-center rounded px-4 text-xs font-semibold text-white disabled:opacity-60"
          >
            {t('ajustes.security.submit')}
          </button>
        </form>
      )}

      {/* Notificaciones push por tipo (expandible) */}
      {showNotifs && prefs && estado.suscrito && (
        <div className="flex flex-col gap-1.5 border-t border-app pt-3">
          <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.notif.tiposTitulo')}</p>
          {TIPOS_NOTIF_USUARIO.map((tipo) => (
            <div key={tipo} className="flex items-center justify-between rounded-lg bg-surface px-3 py-2">
              <span className="text-sm text-app">{t(`ajustes.notif.tipos.${tipo}`)}</span>
              <Switch
                checked={prefs[tipo] !== false}
                onCheckedChange={(checked) => cambiarPref(tipo, checked)}
              />
            </div>
          ))}
        </div>
      )}
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
    </div>
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

// ── § Acerca de ──────────────────────────────────────────────────────────────

const REPO_URL = 'https://github.com/gnacho/helios';

function compareSemver(a: string, b: string): number {
  const pa = a.replace(/^v/, '').split('.').map((n) => parseInt(n, 10) || 0);
  const pb = b.replace(/^v/, '').split('.').map((n) => parseInt(n, 10) || 0);
  for (let i = 0; i < 3; i++) if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) - (pb[i] || 0);
  return 0;
}

function AboutSection() {
  const { t } = useTranslation();
  const [systemInfo, setSystemInfo] = useState<{
    app: { version: string; name: string };
    node: { version: string; platform: string; arch: string };
    react: string;
    uptime: number;
    memory: { rss: number; heapUsed: number };
  } | null>(null);

  useEffect(() => {
    apiFetch<{
      app: { version: string; name: string };
      node: { version: string; platform: string; arch: string };
      react: string;
      uptime: number;
      memory: { rss: number; heapUsed: number };
    }>('/api/system/info')
      .then((data) => setSystemInfo(data))
      .catch(() => {});
  }, []);

  const tiles = [
    { icon: 'github' as const, label: t('ajustes.about.code'), href: REPO_URL },
    { icon: 'file' as const, label: t('ajustes.about.changes'), href: `${REPO_URL}/commits/main` },
    { icon: 'heart' as const, label: t('ajustes.about.kofi'), href: null },
    { icon: 'shield' as const, label: t('ajustes.about.privacy'), href: null },
  ];

  return (
    <div className="grid gap-6 md:grid-cols-2">
      {/* Izquierda: Logo + nombre + versión + descripción */}
      <div className="flex items-start gap-4">
        <motion.div
          initial={{ scale: 0.8, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          transition={{ type: 'spring', stiffness: 260, damping: 20 }}
        >
          <BrandLogo className="h-12 w-12" />
        </motion.div>
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <span className="font-display text-xl font-bold text-app">Helios</span>
            <span className="font-mono text-xs text-faint">v{systemInfo?.app.version || pkg.version}</span>
          </div>
          <p className="mt-1.5 text-sm leading-relaxed text-muted">
            {t('ajustes.about.desc')}
          </p>
          {systemInfo && (
            <div className="mt-3 flex flex-wrap gap-3 text-[11px] text-faint">
              <span>React {systemInfo.react}</span>
              <span>·</span>
              <span>Node {systemInfo.node.version}</span>
              <span>·</span>
              <span>{systemInfo.memory.rss} MB</span>
              <span>·</span>
              <span>Uptime {Math.floor(systemInfo.uptime / 3600)}h {Math.floor((systemInfo.uptime % 3600) / 60)}m</span>
            </div>
          )}
        </div>
      </div>

      {/* Derecha: Tiles de enlaces */}
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {tiles.map((tile, i) => {
          const Icon = tile.icon === 'github' ? Github : tile.icon === 'file' ? FileText : tile.icon === 'heart' ? Heart : ShieldCheck;
          const content = (
            <div className="flex items-center gap-2.5 rounded-xl border border-app px-3.5 py-2.5 text-sm text-muted transition-colors hover:border-brand/40 hover:text-brand">
              <Icon size={16} className="shrink-0" />
              <span>{tile.label}</span>
            </div>
          );
          return tile.href ? (
            <motion.a
              key={tile.label}
              href={tile.href}
              target="_blank"
              rel="noreferrer"
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.3, delay: i * 0.05 }}
            >
              {content}
            </motion.a>
          ) : (
            <motion.div
              key={tile.label}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.3, delay: i * 0.05 }}
            >
              {content}
            </motion.div>
          );
        })}
      </div>
    </div>
  );
}

// ── Página ───────────────────────────────────────────────────────────────────

export default function Ajustes() {
  const { t } = useTranslation();
  const { state: installState, install } = useInstallPrompt();
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

  useEffect(() => {
    lastTickAt.current = Date.now();
    const id = window.setInterval(() => setDataOk(Date.now() - lastTickAt.current < 15000), 5000);
    return () => window.clearInterval(id);
  }, [liveTick]);

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      <motion.header initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.4, ease: 'easeOut' }}>
        <h1 className="font-display text-2xl font-semibold tracking-[-0.01em] text-app">{t('ajustes.title')}</h1>
        <p className="text-sm text-muted">{t('ajustes.subtitle')}</p>
      </motion.header>

      <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-5">

        {/* 1. Tu instalación | Apariencia (alturas igualadas: la fila estira ambas tarjetas) */}
        <div className="grid gap-5 lg:grid-cols-12">
          <div className="lg:col-span-7">
            <Section id="instalacion" title={t('ajustes.sections.instalacion')} className="h-full">
              <InstallationSection />
            </Section>
          </div>
          <div className="lg:col-span-5">
            <Section id="apariencia" title={t('ajustes.sections.apariencia')} className="h-full">
              <ThemeSection />
            </Section>
          </div>
        </div>

        {/* 2. Conexión y datos (fila horizontal propia, span-12) */}
        <Section
          id="conexion"
          title={t('ajustes.sections.conexionDatos')}
          badge={<HealthBadge ok={connectionStatus === 'connected' && dataOk} />}
        >
          <ConnectionSection />
        </Section>

        {/* 3. Mi perfil (span-12) */}
        <Section id="perfil" title={t('ajustes.sections.perfil')}>
          <ProfileSection />
        </Section>

        {/* 3. Zona administración (solo admin): barra AdminBar con Usuarios desplegable */}
        {userRole === 'admin' && <AdminZone />}

        {/* 4. Precios (abajo, en una única fila) */}
        <Section id="precios" title={t('ajustes.sections.precios')}>
          <PricesSection />
        </Section>

        {/* 6. Instalar app + Acerca de */}
        {installState !== 'hidden' ? (
          <>
            <Section id="app" title={t('ajustes.sections.app')}>
              <InstallSection state={installState} install={install} />
            </Section>
            <Section id="acerca" title={t('ajustes.sections.acerca')}>
              <AboutSection />
            </Section>
          </>
        ) : (
          <Section id="acerca" title={t('ajustes.sections.acerca')}>
            <AboutSection />
          </Section>
        )}
      </div>
    </div>
  );
}
