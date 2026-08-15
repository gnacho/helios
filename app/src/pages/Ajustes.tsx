import { useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';
import {
  BatteryCharging,
  Bell,
  Car,
  CarFront,
  Check,
  ChevronDown,
  FileText,
  Github,
  Heart,
  HeartPulse,
  House,
  KeyRound,
  Languages,
  LayoutGrid,
  LogOut,
  Mail,
  MapPin,
  Moon,
  Pencil,
  Puzzle,
  RefreshCw,
  Settings2,
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
import { useInstall } from '@/hooks/useInstall';
import { useExtensions, invalidateExtensions } from '@/hooks/useExtensions';
import type { ExtensionsConfig } from '@/data/types';
import { TopologyEditor } from '@/components/TopologyEditor';
import { THEME_BG, THEME_SURFACE, THEME_BAR, ACCENTS } from '@/lib/colors';
import { useEnergySettings } from '@/hooks/useEnergySettings';
import { usePush } from '@/hooks/usePush';
import BrandLogo from '@/components/BrandLogo';
import { heliosToast } from '@/lib/toast';
import { LANG_MODE_KEY, resolveNavigatorLanguage, numLocale } from '@/i18n';
import { fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';
import { ApiError, apiDelete, apiFetch, apiPost, apiPut } from '@/data/api-client';
import { applyRelease } from '@/data/apply-update';
import pkg from '../../package.json';

const easeOutQuart = [0.25, 1, 0.5, 1] as [number, number, number, number];

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
      const done = await applyRelease();
      if (done) window.location.reload();
      else {
        setApplying(false);
        setUpdateStatus('error');
      }
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
      <div className="flex h-[80px] w-full overflow-hidden rounded-lg border border-app">
        <PreviewBlock useLight={false} />
        <PreviewBlock useLight />
      </div>
    );
  }

  return (
    <div className="flex h-[80px] w-full flex-col rounded-lg border border-app p-1.5" style={{ backgroundColor: bg }}>
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
    <div className="flex flex-col gap-4 sm:flex-row sm:gap-6">
      {/* Tema: radiogroup con mini-previews (máx 50% ancho) */}
      <div role="radiogroup" aria-label={t('theme.label')} className="grid grid-cols-3 gap-2 sm:w-1/2 sm:flex-shrink-0">
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

      {/* Resto de controles */}
      <div className="flex flex-col gap-3 sm:flex-1">
        {/* Acento y Animaciones en línea */}
        <div>
          <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.accent.title')}</p>
          <div className="flex items-center gap-3">
            <div role="radiogroup" aria-label={t('ajustes.accent.title')} className="flex items-center gap-2">
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
                      'relative flex h-7 w-7 items-center justify-center rounded-full transition-transform hover:scale-110',
                      active && 'ring-2 ring-brand ring-offset-2 ring-offset-[var(--surface)]',
                    )}
                    style={{ backgroundColor: acc.hex }}
                  >
                    {active && <Check size={12} strokeWidth={3} className="text-white" />}
                  </button>
                );
              })}
            </div>
            <div className="ml-auto flex items-center gap-2">
              <span className="text-[13px] text-muted">{t('ajustes.reduceMotion')}</span>
              <Switch checked={!reduceMotion} onCheckedChange={(v) => setReduceMotion(!v)} />
            </div>
          </div>
        </div>

        {/* Densidad */}
        <div>
          <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.density.title')}</p>
          <div role="radiogroup" aria-label={t('ajustes.density.title')} className="flex rounded-xl border border-app bg-surface p-0.5">
            {(['comfortable', 'compact'] as const).map((d) => (
              <button
                key={d}
                type="button"
                role="radio"
                aria-checked={density === d}
                onClick={() => setDensity(d)}
                className={cn(
                  'h-8 flex-1 rounded-lg text-[13px] transition-colors',
                  density === d ? 'bg-surface-2 font-semibold text-app shadow-soft' : 'text-faint hover:text-muted',
                )}
              >
                {t(`ajustes.density.${d}`)}
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

// ── §3 Conexión con Home Assistant ───────────────────────────────────────────

function ConnectionSection({ isAdmin = false }: { isAdmin?: boolean }) {
  const { connectionStatus, getLivePower } = useEnergyData();
  const { t } = useTranslation();
  const install = useInstall();
  const [testing, setTesting] = useState(false);
  const [editorOpen, setEditorOpen] = useState(false);
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

  // Descripción por rol: las entidades vienen resueltas del backend (topología),
  // no hardcodeadas. Los roles legacy mantienen su traducción; los roles de
  // inversores genéricos usan el nombre del inversor.
  const descKeyFor = (role: string, name?: string) => {
    const legacy: Record<string, string> = {
      inverter: name ? `desc_inverter` : 'desc_inverter',
      inverter_energy: name ? `desc_inverter_energy` : 'desc_inverter_energy',
      consumption: 'desc_consumption',
      battery_power: 'desc_battery_power',
      battery_soc: 'desc_solis_bateria_soc',
      battery_state: 'desc_battery_state',
      grid_scraper: 'desc_grid_attrs',
      grid_attrs: 'desc_grid_attrs',
      grid_sensor: 'desc_grid_sensor',
      grid_import: 'desc_grid_import',
      grid_export: 'desc_grid_export',
      inverter_status: 'desc_inverter_status',
      sun: 'desc_sun',
      weather: 'desc_weather',
      weather_temp: 'desc_weather_temp',
    };
    const key = legacy[role] || 'desc_consumption';
    return t(`ajustes.connection.${key}`, name ? { name } : undefined);
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
        {isAdmin && install && (
          <button
            type="button"
            onClick={() => setEditorOpen(true)}
            title={install.configured
              ? t('ajustes.topology.configured')
              : t('ajustes.topology.notConfigured')}
            className="ml-auto inline-flex h-9 items-center gap-1.5 rounded-full border border-app bg-surface px-4 text-[13px] font-medium text-app transition-colors hover:border-brand/50 hover:text-brand"
          >
            <Pencil size={13} />
            {t('ajustes.topology.edit')}
          </button>
        )}
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
                {(install?.entities ?? []).map((e, i) => (
                  <TableRow key={`${e.role}-${e.entidad}-${i}`}>
                    <TableCell className="py-2 font-mono text-xs text-app">{e.entidad}</TableCell>
                    <TableCell className="py-2 text-xs text-muted">{descKeyFor(e.role, e.name)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </AccordionContent>
        </AccordionItem>
      </Accordion>
      <TopologyEditor open={editorOpen} onOpenChange={setEditorOpen} install={install} />
    </div>
  );
}

// ── §3b Extensiones (issue #94): barra con interruptor maestro + módulos ─────

/** Campo de entidad HAOS del cargador (label + input mono). */
function EntityField({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  const id = `ext-${label.replace(/\s+/g, '-').toLowerCase()}`;
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className="text-xs text-muted">
        {label}
      </Label>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-9 font-mono text-[13px]"
        spellCheck={false}
      />
    </div>
  );
}

function ExtensionsSection() {
  const { t } = useTranslation();
  const ext = useExtensions();
  const [draft, setDraft] = useState<ExtensionsConfig | null>(null);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  // Opciones PLEGADAS por defecto: se despliegan con el icono de config
  // (solo visible con el componente aplicado). Un estado por módulo.
  const [configOpen, setConfigOpen] = useState(false);
  const [bydConfigOpen, setBydConfigOpen] = useState(false);

  // La draft se inicializa desde la config resuelta (y se refresca si aún no
  // había cargado). Guardar escribe { enabled, carCharger, byd } completos.
  useEffect(() => {
    if (ext && !draft) setDraft({ ...ext, chargerActive: undefined, bydActive: undefined });
  }, [ext, draft]);

  // Al desactivar un módulo se vuelve a plegar (estado por defecto).
  useEffect(() => {
    if (!draft?.carCharger?.enabled) setConfigOpen(false);
  }, [draft?.carCharger?.enabled]);
  useEffect(() => {
    if (!draft?.byd?.enabled) setBydConfigOpen(false);
  }, [draft?.byd?.enabled]);

  if (!ext || !draft) {
    return <p className="text-sm text-faint">…</p>;
  }

  const save = async (next: ExtensionsConfig) => {
    if (saving) return;
    setDraft(next);
    setSaving(true);
    setSaved(false);
    try {
      await apiPut<{ ok: boolean; restartNeeded?: boolean }>('/api/extensions', {
        enabled: next.enabled,
        carCharger: next.carCharger,
        byd: next.byd,
      });
      invalidateExtensions();
      setSaved(true);
      window.setTimeout(() => setSaved(false), 2500);
      heliosToast(t('ajustes.extensions.savedRestart'), { tone: 'warning' });
    } catch (err) {
      heliosToast(err instanceof ApiError ? err.message : t('common.error'), { tone: 'warning' });
    } finally {
      setSaving(false);
    }
  };

  const toggleMaster = (checked: boolean) => void save({ ...draft, enabled: checked });
  const toggleCharger = (checked: boolean) =>
    void save({ ...draft, carCharger: { ...draft.carCharger, enabled: checked } });
  const toggleByd = (checked: boolean) =>
    void save({ ...draft, byd: { ...draft.byd, enabled: checked } });

  const ch = draft.carCharger;
  const setCh = (patch: Partial<ExtensionsConfig['carCharger']>) =>
    setDraft({ ...draft, carCharger: { ...ch, ...patch } });
  const byd = draft.byd;
  const setByd = (patch: Partial<ExtensionsConfig['byd']>) =>
    setDraft({ ...draft, byd: { ...byd, ...patch } });

  const configBtn = (open: boolean, onClick: () => void, controls: string) => (
    <button
      type="button"
      onClick={onClick}
      aria-expanded={open}
      aria-controls={controls}
      aria-label={t('ajustes.extensions.configure')}
      title={t('ajustes.extensions.configure')}
      className={cn(
        'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border transition-colors',
        open
          ? 'border-brand bg-brand/10 text-brand'
          : 'border-app bg-surface text-muted hover:bg-surface-2 hover:text-app',
      )}
    >
      <Settings2 size={15} strokeWidth={1.75} />
    </button>
  );

  const bydField = (key: keyof ExtensionsConfig['byd'], labelKey: string, ph = '') => (
    <EntityField
      label={t(labelKey)}
      value={byd[key] as string}
      onChange={(v) => setByd({ [key]: v } as Partial<ExtensionsConfig['byd']>)}
      placeholder={ph}
    />
  );

  return (
    <div className="flex flex-col gap-4">
      {/* Barra horizontal: interruptor maestro + módulos en la misma fila */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-3">
        <div className="flex items-center gap-2.5">
          <Puzzle size={18} strokeWidth={1.75} className="shrink-0 text-brand" />
          <span className="text-sm font-semibold text-app">{t('ajustes.extensions.enable')}</span>
          <Switch checked={draft.enabled} onCheckedChange={toggleMaster} disabled={saving} aria-label={t('ajustes.extensions.enable')} />
        </div>
        {draft.enabled && (
          <>
            <div className="hidden h-6 w-px bg-app sm:block" />
            <div className="flex items-center gap-2.5">
              <CarFront size={18} strokeWidth={1.75} className="shrink-0 text-brand" />
              <span className="text-sm font-semibold text-app">{t('ajustes.extensions.carCharger')}</span>
              <Switch checked={ch.enabled} onCheckedChange={toggleCharger} disabled={saving} aria-label={t('ajustes.extensions.carCharger')} />
              {ch.enabled && configBtn(configOpen, () => setConfigOpen((v) => !v), 'ext-charger-config')}
            </div>
            <div className="hidden h-6 w-px bg-app sm:block" />
            <div className="flex items-center gap-2.5">
              <Car size={18} strokeWidth={1.75} className="shrink-0 text-brand" />
              <span className="text-sm font-semibold text-app">{t('ajustes.extensions.byd')}</span>
              <Switch checked={byd.enabled} onCheckedChange={toggleByd} disabled={saving} aria-label={t('ajustes.extensions.byd')} />
              {byd.enabled && configBtn(bydConfigOpen, () => setBydConfigOpen((v) => !v), 'ext-byd-config')}
            </div>
          </>
        )}
        {saving && <RefreshCw size={14} className="animate-spin text-faint" />}
        {saved && !saving && <Check size={14} className="text-emerald-500" />}
      </div>

      {!draft.enabled ? (
        <p className="text-sm text-muted">{t('ajustes.extensions.offHint')}</p>
      ) : !ch.enabled && !byd.enabled ? (
        <p className="text-sm text-muted">{t('ajustes.extensions.carChargerOffHint')}</p>
      ) : (
        <AnimatePresence initial={false}>
          {configOpen && ch.enabled && (
            <motion.div
              key="ext-charger-config"
              id="ext-charger-config"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.25, ease: 'easeOut' }}
              className="overflow-hidden"
            >
              <div className="flex flex-col gap-4">
                <p className="text-sm text-muted">{t('ajustes.extensions.carChargerDesc')}</p>
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  <EntityField label={t('ajustes.extensions.f.name')} value={ch.name} onChange={(v) => setCh({ name: v })} />
                  <div className="flex flex-col gap-1.5">
                    <Label className="text-xs text-muted">{t('ajustes.extensions.f.powerUnit')}</Label>
                    <Select value={ch.powerUnit} onValueChange={(v: 'kW' | 'W') => setCh({ powerUnit: v })}>
                      <SelectTrigger className="h-9 w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="kW">kW</SelectItem>
                        <SelectItem value="W">W</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <EntityField label={t('ajustes.extensions.f.chargingStates')} value={ch.chargingStates.join(', ')} onChange={(v) => setCh({ chargingStates: v.split(',').map((s) => s.trim()).filter(Boolean) })} placeholder="charging" />
                  <EntityField label={t('ajustes.extensions.f.powerId')} value={ch.powerId} onChange={(v) => setCh({ powerId: v })} />
                  <EntityField label={t('ajustes.extensions.f.energyTotalId')} value={ch.energyTotalId} onChange={(v) => setCh({ energyTotalId: v })} />
                  <EntityField label={t('ajustes.extensions.f.energyDivisor')} value={String(ch.energyDivisor)} onChange={(v) => setCh({ energyDivisor: Math.max(1, Math.min(100000, Math.round(Number(v) || 1))) })} placeholder="1" />
            <div className="flex flex-col gap-1.5">
              <Label className="text-xs text-muted">{t('ajustes.extensions.f.chargerInHouseMeters')}</Label>
              <div className="flex h-9 items-center gap-2">
                <Switch checked={ch.chargerInHouseMeters} onCheckedChange={(v) => setCh({ chargerInHouseMeters: v })} aria-label={t('ajustes.extensions.f.chargerInHouseMeters')} />
                <span className="text-xs text-faint">{t('ajustes.extensions.f.chargerInHouseMetersHint')}</span>
              </div>
            </div>
                  <EntityField label={t('ajustes.extensions.f.energySessionId')} value={ch.energySessionId} onChange={(v) => setCh({ energySessionId: v })} />
                  <EntityField label={t('ajustes.extensions.f.stateId')} value={ch.stateId} onChange={(v) => setCh({ stateId: v })} />
                  <EntityField label={t('ajustes.extensions.f.tempId')} value={ch.tempId} onChange={(v) => setCh({ tempId: v })} />
                  <EntityField label={t('ajustes.extensions.f.switchId')} value={ch.switchId} onChange={(v) => setCh({ switchId: v })} />
                  <EntityField label={t('ajustes.extensions.f.connectedStates')} value={ch.connectedStates.join(', ')} onChange={(v) => setCh({ connectedStates: v.split(',').map((s) => s.trim()).filter(Boolean) })} placeholder="connected" />
                </div>
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => void save(draft)}
                    disabled={saving}
                    className="inline-flex h-9 items-center gap-2 rounded-lg bg-brand-gradient px-4 text-[13px] font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-60"
                  >
                    {t('ajustes.extensions.save')}
                  </button>
                  <p className="text-xs text-faint">{t('ajustes.extensions.entitiesHint')}</p>
                </div>
              </div>
            </motion.div>
          )}

          {bydConfigOpen && byd.enabled && (
            <motion.div
              key="ext-byd-config"
              id="ext-byd-config"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.25, ease: 'easeOut' }}
              className="overflow-hidden"
            >
              <div className="flex flex-col gap-4">
                <p className="text-sm text-muted">{t('ajustes.extensions.bydDesc')}</p>
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {bydField('name', 'ajustes.extensions.f.name')}
                  {bydField('socId', 'ajustes.extensions.f_byd.soc')}
                  {bydField('rangeId', 'ajustes.extensions.f_byd.range')}
                  {bydField('odometerId', 'ajustes.extensions.f_byd.odometer')}
                  {bydField('batteryPowerId', 'ajustes.extensions.f_byd.batteryPower')}
                  {bydField('cabinTempId', 'ajustes.extensions.f_byd.cabinTemp')}
                  {bydField('exteriorTempId', 'ajustes.extensions.f_byd.exteriorTemp')}
                  {bydField('chargingId', 'ajustes.extensions.f_byd.charging')}
                  {bydField('pluggedId', 'ajustes.extensions.f_byd.plugged')}
                  {bydField('onlineId', 'ajustes.extensions.f_byd.online')}
                  {bydField('lockedId', 'ajustes.extensions.f_byd.locked')}
                  {bydField('doorsId', 'ajustes.extensions.f_byd.doors')}
                  {bydField('windowsId', 'ajustes.extensions.f_byd.windows')}
                  {bydField('sentryId', 'ajustes.extensions.f_byd.sentry')}
                  {bydField('tireFlId', 'ajustes.extensions.f_byd.tireFl')}
                  {bydField('tireFrId', 'ajustes.extensions.f_byd.tireFr')}
                  {bydField('tireRlId', 'ajustes.extensions.f_byd.tireRl')}
                  {bydField('tireRrId', 'ajustes.extensions.f_byd.tireRr')}
                  {bydField('locationId', 'ajustes.extensions.f_byd.location')}
                  {bydField('gpsAgeId', 'ajustes.extensions.f_byd.gpsAge')}
                  {bydField('lastUpdateId', 'ajustes.extensions.f_byd.lastUpdate')}
                  {bydField('startChargeId', 'ajustes.extensions.f_byd.startCharge')}
                  {bydField('stopChargeId', 'ajustes.extensions.f_byd.stopCharge')}
                  {bydField('forcePollId', 'ajustes.extensions.f_byd.forcePoll')}
                  {bydField('chargeToFullId', 'ajustes.extensions.f_byd.chargeToFull')}
                  {bydField('scheduleEnabledId', 'ajustes.extensions.f_byd.scheduleEnabled')}
                  {bydField('scheduleStartId', 'ajustes.extensions.f_byd.scheduleStart')}
                  {bydField('scheduleEndId', 'ajustes.extensions.f_byd.scheduleEnd')}
                  {bydField('repeatDailyId', 'ajustes.extensions.f_byd.repeatDaily')}
                </div>
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => void save(draft)}
                    disabled={saving}
                    className="inline-flex h-9 items-center gap-2 rounded-lg bg-brand-gradient px-4 text-[13px] font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-60"
                  >
                    {t('ajustes.extensions.save')}
                  </button>
                  <p className="text-xs text-faint">{t('ajustes.extensions.entitiesHint')}</p>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      )}
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

const LEGACY_INVERTERS = [
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

// Parse de "N × W W" (panels de la topología) → {panels, panelW}. Fallback 0.
function parsePanels(s: string): { panels: number; panelW: number } {
  const m = s.match(/(\d+)\s*×\s*(\d+)\s*W/i);
  if (!m) return { panels: 0, panelW: 0 };
  return { panels: Number(m[1]), panelW: Number(m[2]) };
}

function fmtKwp(v: number) {
  return new Intl.NumberFormat(numLocale(), { maximumFractionDigits: 1 }).format(v);
}

function InstallationSection() {
  const install = useInstall();
  const [settings, update] = useEnergySettings();
  const { t } = useTranslation();

  // Inversores desde la topología resuelta (issue #37); fallback al clásico
  // Solis/Fox solo si la API no ha respondido aún.
  const inverters = useMemo(() => {
    if (install && install.inverters.length > 0) {
      return install.inverters.map((inv) => {
        const p = parsePanels(inv.panels || '');
        return {
          key: inv.key,
          name: inv.model || inv.name,
          color: inv.key === 'solis' ? 'var(--c-solis)' : inv.key === 'fox' ? 'var(--c-fox)' : 'var(--brand)',
          panels: p.panels,
          panelW: p.panelW,
          kwp: inv.kwp,
          battery: inv.hasBattery ? { name: t('common.battery'), kwh: inv.batteryKwh } : null,
        };
      });
    }
    return LEGACY_INVERTERS.map((inv) => ({ ...inv }));
  }, [install, t]);

  const totalKwp = inverters.reduce((acc, inv) => acc + inv.kwp, 0);
  const totalPanels = inverters.reduce((acc, inv) => acc + inv.panels, 0);

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 md:grid-cols-3">
        {inverters.map((inv, i) => (
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
              {inv.panels > 0 && (
                <p className="flex items-center gap-2 text-[13px] text-muted">
                  <LayoutGrid size={15} className="shrink-0 text-faint" />
                  {t('ajustes.install.panels', { count: inv.panels, watts: inv.panelW })}
                </p>
              )}
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
              {fmtKwp(totalKwp)} <span className="text-base font-medium text-faint">kWp</span>
            </p>

            <div className="mt-3 flex flex-col gap-2">
              <p className="flex items-center gap-2 text-[13px] text-muted">
                <Zap size={15} className="shrink-0 text-faint" />
                {inverters.length} {t('ajustes.install.inverters')}
              </p>
              {totalPanels > 0 && (
                <p className="flex items-center gap-2 text-[13px] text-muted">
                  <LayoutGrid size={15} className="shrink-0 text-faint" />
                  {totalPanels} {t('ajustes.install.panelsShort')}
                </p>
              )}
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
  'bateria_llena',
  'resumen_diario',
] as const;

function ProfileSection() {
  const { t, i18n } = useTranslation();
  const { estado, soporte, activar } = usePush();
  const [user, setUser] = useState<{ id: string; username: string; display_name?: string | null; email?: string | null; language: string; role?: string } | null>(null);
  const [showPwd, setShowPwd] = useState(false);
  const [showNotifs, setShowNotifs] = useState(false);
  const [prefs, setPrefs] = useState<Record<string, boolean> | null>(null);
  const [langError, setLangError] = useState<string | null>(null);

  const [editingName, setEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState('');
  const [nameBusy, setNameBusy] = useState(false);
  const [nameError, setNameError] = useState<string | null>(null);

  const [editingEmail, setEditingEmail] = useState(false);
  const [emailDraft, setEmailDraft] = useState('');
  const [emailBusy, setEmailBusy] = useState(false);
  const [emailError, setEmailError] = useState<string | null>(null);

  const [pwdCurrent, setPwdCurrent] = useState('');
  const [pwdNew, setPwdNew] = useState('');
  const [pwdConfirm, setPwdConfirm] = useState('');
  const [pwdBusy, setPwdBusy] = useState(false);
  const [pwdError, setPwdError] = useState<string | null>(null);

  const languages = [
    { code: 'auto', name: t('ajustes.language.auto'), flag: '🌐' },
    { code: 'es', name: 'Español', flag: '🇪🇸' },
    { code: 'en', name: 'English', flag: '🇬🇧' },
    { code: 'zh-CN', name: '简体中文', flag: '🇨🇳' },
  ];

  const isAuto = localStorage.getItem(LANG_MODE_KEY) === 'auto';
  const langValue = isAuto
    ? 'auto'
    : i18n.language.startsWith('en')
      ? 'en'
      : i18n.language.startsWith('zh')
        ? 'zh-CN'
        : 'es';

  useEffect(() => {
    apiFetch<{ user?: { id: string; username: string; display_name?: string | null; email?: string | null; language: string; role?: string } }>('/api/auth/profile')
      .then((d) => {
        if (d.user) {
          setUser(d.user);
          setNameDraft(d.user.display_name || d.user.username);
          setEmailDraft(d.user.email || '');
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => { if (!editingName) setNameDraft(user?.display_name || user?.username || ''); }, [user?.display_name, user?.username, editingName]);
  useEffect(() => { if (!editingEmail) setEmailDraft(user?.email || ''); }, [user?.email, editingEmail]);

  useEffect(() => {
    apiFetch<{ prefs: Record<string, boolean> }>('/api/push/preferences')
      .then((r) => setPrefs(r.prefs))
      .catch(() => setPrefs(null));
  }, []);

  const handleLang = (value: string) => {
    setLangError(null);
    if (value === 'auto') {
      localStorage.setItem(LANG_MODE_KEY, 'auto');
      i18n.changeLanguage(resolveNavigatorLanguage());
    } else {
      localStorage.setItem(LANG_MODE_KEY, 'manual');
      i18n.changeLanguage(value);
    }
    apiPut('/api/auth/profile', { language: value === 'auto' ? null : value }).catch(() => {
      setLangError(t('ajustes.profile.profileError'));
    });
  };

  const saveName = async () => {
    const value = nameDraft.trim();
    if (value === (user?.display_name || '')) { setEditingName(false); return; }
    setNameBusy(true); setNameError(null);
    try {
      await apiPut('/api/auth/profile', { display_name: value || null });
      setUser((u) => u ? { ...u, display_name: value || null } : u);
      heliosToast(t('ajustes.profile.saved'), { tone: 'success' });
      setEditingName(false);
    } catch {
      setNameError(t('ajustes.profile.profileError'));
    } finally { setNameBusy(false); }
  };

  const saveEmail = async () => {
    const value = emailDraft.trim();
    if (value === (user?.email || '')) { setEditingEmail(false); return; }
    setEmailBusy(true); setEmailError(null);
    try {
      await apiPut('/api/auth/profile', { email: value || null });
      setUser((u) => u ? { ...u, email: value || null } : u);
      setEditingEmail(false);
    } catch {
      setEmailError(t('ajustes.profile.profileError'));
    } finally { setEmailBusy(false); }
  };

  const cancelName = () => { setNameDraft(user?.display_name || user?.username || ''); setNameError(null); setEditingName(false); };
  const cancelEmail = () => { setEmailDraft(user?.email || ''); setEmailError(null); setEditingEmail(false); };

  const cambiarPref = (tipo: string, enabled: boolean) => {
    setPrefs((p) => (p ? { ...p, [tipo]: enabled } : p));
    apiPut('/api/push/preferences', { tipo, enabled }).catch(() => {
      setPrefs((p) => (p ? { ...p, [tipo]: !enabled } : p));
    });
  };

  const doLogout = async () => {
    try { await apiPost('/api/auth/logout'); } finally {
      window.dispatchEvent(new Event('helios-unauthorized'));
    }
  };

  const submitPwd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (pwdBusy || pwdNew !== pwdConfirm || pwdNew.length < 6) return;
    setPwdBusy(true); setPwdError(null);
    try {
      await apiPut('/api/auth/password', { current: pwdCurrent, password: pwdNew });
      heliosToast(t('ajustes.security.changed'), { tone: 'success' });
      setShowPwd(false);
      setPwdCurrent(''); setPwdNew(''); setPwdConfirm('');
    } catch (err) {
      setPwdError(err instanceof ApiError ? err.message : t('common.error'));
    } finally { setPwdBusy(false); }
  };

  const displayName = user?.display_name || user?.username || '';
  const actionBtnCls = 'inline-flex h-9 items-center gap-1.5 rounded-lg border border-app px-2.5 sm:px-3 text-[13px] font-medium text-faint transition-colors hover:bg-surface-2 hover:text-app';
  const actionTextCls = 'hidden sm:inline';

  return (
    <>
      <div className="flex flex-wrap items-center gap-3 sm:gap-4 min-w-0">
        {/* Avatar */}
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-brand/12 text-brand">
          <User className="h-5 w-5" />
        </div>

        {/* Nombre + rol */}
        <div className="min-w-0 flex-1">
          {editingName ? (
            <div className="flex items-center gap-2">
              <input
                type="text" value={nameDraft}
                onChange={(e) => setNameDraft(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') void saveName(); if (e.key === 'Escape') cancelName(); }}
                disabled={nameBusy}
                className="h-9 w-full rounded-xl border border-app bg-surface-2 px-3 py-1 text-[15px] text-app outline-none focus:border-brand"
                placeholder={user?.username}
                autoFocus
              />
              <button type="button" onClick={() => void saveName()} disabled={nameBusy} aria-label={t('common.save')}
                className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-brand text-white transition-colors hover:brightness-110 disabled:opacity-50">
                <Check className="h-4 w-4" />
              </button>
              <button type="button" onClick={cancelName} disabled={nameBusy} aria-label={t('common.cancel')}
                className="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-app bg-surface-2 text-faint transition-colors hover:bg-surface hover:text-app">
                <X className="h-4 w-4" />
              </button>
            </div>
          ) : (
            <button type="button" onClick={() => setEditingName(true)} className="group flex items-center gap-1.5 min-w-0" title={t('ajustes.profile.editName')}>
              <p className="text-base font-semibold leading-tight truncate">{displayName || user?.username || ''}</p>
              <Pencil className="h-3.5 w-3.5 text-faint opacity-0 group-hover:opacity-100 transition-opacity" />
            </button>
          )}
          {user?.role === 'admin' && <p className="text-[13px] text-faint leading-tight mt-0.5">{t('ajustes.role.admin', { defaultValue: 'Administrador' })}</p>}
          {nameError && <p role="alert" className="text-xs text-destructive mt-1">{nameError}</p>}
        </div>

        {/* Email + acciones */}
        <div className="flex flex-wrap items-center gap-2 sm:gap-3 min-w-0">
          {editingEmail ? (
            <div className="flex items-center gap-2">
              <input
                type="email" value={emailDraft}
                onChange={(e) => setEmailDraft(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') void saveEmail(); if (e.key === 'Escape') cancelEmail(); }}
                disabled={emailBusy}
                className="h-9 w-[180px] sm:w-[220px] rounded-xl border border-app bg-surface-2 px-3 py-1 text-[13px] text-app outline-none focus:border-brand"
                placeholder={t('ajustes.profile.emailPlaceholder')}
                autoFocus
              />
              <button type="button" onClick={() => void saveEmail()} disabled={emailBusy} aria-label={t('common.save')}
                className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-brand text-white transition-colors hover:brightness-110 disabled:opacity-50">
                <Check className="h-4 w-4" />
              </button>
              <button type="button" onClick={cancelEmail} disabled={emailBusy} aria-label={t('common.cancel')}
                className="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-app bg-surface-2 text-faint transition-colors hover:bg-surface hover:text-app">
                <X className="h-4 w-4" />
              </button>
            </div>
          ) : (
            <button type="button" onClick={() => setEditingEmail(true)}
              className={cn('inline-flex h-9 w-9 items-center justify-center rounded-full transition-colors',
                user?.email
                  ? 'bg-amber-500/10 text-amber-500 hover:bg-amber-500/20'
                  : 'border border-app text-faint hover:bg-surface-2 hover:text-app')}
              title={user?.email || t('ajustes.profile.addEmail')}
              aria-label={user?.email ? t('ajustes.profile.editEmail') : t('ajustes.profile.addEmail')}>
              <Mail className="h-4 w-4" />
            </button>
          )}

          {/* Idioma — icono en móvil, select completo en desktop */}
          <label htmlFor="profile-lang" className="sm:hidden inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-app text-faint cursor-pointer hover:bg-surface-2 hover:text-app" title={t('ajustes.language.title')}>
            <Languages className="h-4 w-4" />
          </label>
          <select
            id="profile-lang"
            value={langValue}
            onChange={(e) => void handleLang(e.target.value)}
            className="hidden sm:inline h-9 w-[120px] shrink-0 rounded-lg border border-app bg-surface-2 px-2 text-[13px] text-app outline-none focus:border-brand"
          >
            {languages.map((l) => <option key={l.code} value={l.code}>{l.flag} {l.name}</option>)}
          </select>

          {/* Contraseña */}
          <button type="button" aria-expanded={showPwd} onClick={() => setShowPwd((v) => !v)} className={actionBtnCls} title={t('ajustes.security.change')}>
            <KeyRound className="h-4 w-4" />
            <span className={actionTextCls}>{t('common.password')}</span>
          </button>

          {/* Notificaciones */}
          <button type="button" aria-expanded={showNotifs} onClick={() => setShowNotifs((v) => !v)}
            className={cn(actionBtnCls, showNotifs && 'border-brand/30 bg-brand/5 text-brand hover:bg-brand/10')}
            title={t('ajustes.sections.notificaciones')}>
            <Bell className="h-4 w-4" />
            <span className={actionTextCls}>{t('ajustes.sections.notificaciones')}</span>
          </button>
        </div>

        {/* Cerrar sesión */}
        <button type="button" onClick={() => void doLogout()}
          className="ml-auto inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border border-destructive/30 bg-destructive/10 px-2.5 sm:px-3 text-[13px] font-medium text-destructive transition-colors hover:bg-destructive/15">
          <LogOut className="h-4 w-4" />
          <span className={actionTextCls}>{t('common.logout')}</span>
        </button>
      </div>

      {emailError && <p role="alert" className="text-xs text-destructive mt-3">{emailError}</p>}
      {langError && <p role="alert" className="text-xs text-destructive mt-3">{langError}</p>}

      {/* Formulario de contraseña (expandible) */}
      {showPwd && (
        <form onSubmit={submitPwd} className="mt-4 flex flex-col gap-2 border-t border-app pt-4">
          <input type="password" autoComplete="current-password" placeholder={t('ajustes.security.current', { defaultValue: 'Contraseña actual' })}
            value={pwdCurrent} onChange={(e) => setPwdCurrent(e.target.value)}
            className="h-9 rounded-xl border border-app bg-surface-2 px-3 text-sm text-app outline-none focus:border-brand" />
          <div className="grid gap-2 sm:grid-cols-2">
            <input type="password" autoComplete="new-password" placeholder={t('ajustes.security.new', { defaultValue: 'Nueva contraseña' })}
              value={pwdNew} onChange={(e) => setPwdNew(e.target.value)}
              className="h-9 rounded-xl border border-app bg-surface-2 px-3 text-sm text-app outline-none focus:border-brand" />
            <input type="password" autoComplete="new-password" placeholder={t('ajustes.security.confirm', { defaultValue: 'Confirmar' })}
              value={pwdConfirm} onChange={(e) => setPwdConfirm(e.target.value)}
              className="h-9 rounded-xl border border-app bg-surface-2 px-3 text-sm text-app outline-none focus:border-brand" />
          </div>
          {pwdNew.length > 0 && pwdNew !== pwdConfirm && (
            <p className="text-xs text-destructive">{t('ajustes.security.mismatch')}</p>
          )}
          {pwdError && <p role="alert" className="text-xs text-destructive">{pwdError}</p>}
          <button type="submit" disabled={pwdBusy || !pwdCurrent || pwdNew.length < 6 || pwdNew !== pwdConfirm}
            className="bg-brand-gradient inline-flex h-9 self-start items-center rounded-lg px-4 text-sm font-semibold text-white disabled:opacity-60">
            {t('ajustes.security.submit', { defaultValue: 'Cambiar contraseña' })}
          </button>
        </form>
      )}

      {/* Notificaciones push (expandible) */}
      {showNotifs && prefs && (
        <div className="mt-4 flex flex-col gap-1.5 border-t border-app pt-4">
          {estado.suscrito ? (
            <>
              <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('ajustes.notif.tiposTitulo')}</p>
              {TIPOS_NOTIF_USUARIO.map((tipo) => (
                <div key={tipo} className="flex items-center justify-between rounded-lg bg-surface-2 px-3 py-2">
                  <span className="text-sm text-app">{t(`ajustes.notif.tipos.${tipo}`)}</span>
                  <Switch checked={prefs[tipo] !== false} onCheckedChange={(checked) => cambiarPref(tipo, checked)} />
                </div>
              ))}
            </>
          ) : (
            <div className="flex flex-col gap-2 rounded-lg bg-surface-2 px-3 py-2.5">
              <p className="text-sm text-muted">{t('ajustes.notif.descripcion')}</p>
              {soporte === 'ok' ? (
                <button type="button" onClick={() => activar()} disabled={estado.cargando}
                  className="bg-brand-gradient inline-flex h-8 items-center justify-center gap-1.5 self-start rounded px-3 text-xs font-semibold text-white disabled:opacity-60">
                  <Bell size={14} /> {t('ajustes.notif.activar')}
                </button>
              ) : (
                <p className="text-xs text-faint">
                  {soporte === 'requiere-https' ? t('ajustes.notif.requiereHttps')
                    : soporte === 'ios-necesita-instalacion' ? t('ajustes.notif.iosInstalacion')
                    : soporte === 'no-configurado' ? t('ajustes.notif.noConfigurado')
                    : t('ajustes.notif.noSoportado')}
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </>
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
    { icon: 'file' as const, label: t('ajustes.about.changes'), href: 'https://helios.cloudless.club' },
    { icon: 'heart' as const, label: t('ajustes.about.kofi'), href: 'https://ko-fi.com/gnacho' },
    { icon: 'shield' as const, label: t('ajustes.about.privacy'), href: 'https://cloudless.club' },
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

        {/* 1. Tu instalación */}
        <Section id="instalacion" title={t('ajustes.sections.instalacion')}>
          <InstallationSection />
        </Section>

        {/* 1b. Apariencia */}
        <Section id="apariencia" title={t('ajustes.sections.apariencia')}>
          <ThemeSection />
        </Section>

        {/* 2. Conexión y datos (fila horizontal propia, span-12) */}
        <Section
          id="conexion"
          title={t('ajustes.sections.conexionDatos')}
          badge={<HealthBadge ok={connectionStatus === 'connected' && dataOk} />}
        >
          <ConnectionSection isAdmin={userRole === 'admin'} />
        </Section>

        {/* 2b. Extensiones (solo admin): barra maestra + módulo cargador */}
        {userRole === 'admin' && (
          <Section id="extensiones" title={t('ajustes.sections.extensiones')}>
            <ExtensionsSection />
          </Section>
        )}

        {/* 3. Mi perfil (span-12) */}
        <motion.section
          initial={{ opacity: 0, y: 24 }} whileInView={{ opacity: 1, y: 0 }} viewport={{ once: true }}
          transition={{ duration: 0.5, ease: [0.22, 1, 0.36, 1] }}
          className="helios-card rounded-2xl p-5"
        >
          <ProfileSection />
        </motion.section>

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
