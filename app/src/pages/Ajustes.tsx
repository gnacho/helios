import { useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { motion } from 'framer-motion';
import {
  BatteryCharging,
  Check,
  House,
  MapPin,
  Moon,
  RefreshCw,
  Smartphone,
  Sun,
  Sunrise,
  Zap,
} from 'lucide-react';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { useTheme } from '@/theme/ThemeProvider';
import type { ThemeMode } from '@/theme/ThemeProvider';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { useEnergySettings } from '@/hooks/useEnergySettings';
import HeliosToaster from '@/components/HeliosToaster';
import { heliosToast } from '@/lib/toast';
import { fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';

const easeOutQuart = [0.25, 1, 0.5, 1] as [number, number, number, number];

const SECTIONS = [
  { id: 'tema', label: 'Tema' },
  { id: 'conexion', label: 'Conexión' },
  { id: 'instalacion', label: 'Instalación' },
  { id: 'precios', label: 'Precios' },
  { id: 'datos', label: 'Datos' },
  { id: 'app', label: 'App' },
] as const;

const ENTIDADES: { entidad: string; descripcion: string }[] = [
  { entidad: 'sensor.solis_potencia_actual', descripcion: 'Producción Solis (kW)' },
  { entidad: 'sensor.almacen_pinza_power_b', descripcion: 'Producción Fox, pinza local (W)' },
  { entidad: 'sensor.medidor_respaldo_power', descripcion: 'Consumo vivienda respaldada (W)' },
  { entidad: 'sensor.vivienda_medidor_power', descripcion: 'Consumo vivienda no respaldada (W)' },
  { entidad: 'sensor.almacen_pinza_power_a', descripcion: 'Consumo almacén (W)' },
  { entidad: 'sensor.solis_bateria_soc', descripcion: 'Batería Soluna (%)' },
  { entidad: 'sensor.solis_bateria_potencia', descripcion: 'Potencia carga/descarga (kW)' },
  { entidad: 'sensor.solis_scraper', descripcion: 'Red Solis: dirección y potencia (atributos)' },
  { entidad: 'sun.sun', descripcion: 'Amanecer/atardecer' },
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

const THEME_OPTIONS: { value: ThemeMode; label: string; icon: typeof Sun }[] = [
  { value: 'auto', label: 'Auto', icon: Sunrise },
  { value: 'light', label: 'Claro', icon: Sun },
  { value: 'dark', label: 'Oscuro', icon: Moon },
];

const THEME_PREVIEWS = [
  {
    key: 'light',
    label: 'Claro',
    bg: '#F4F6FA',
    surface: '#FFFFFF',
    text: '#0C1425',
    faint: '#94A3B8',
    border: '#E3E8F0',
    accent: '#F59E0B',
  },
  {
    key: 'dark',
    label: 'Oscuro',
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

  return (
    <div className="flex flex-col gap-4">
      {/* Segmented grande */}
      <div role="radiogroup" aria-label="Tema de la interfaz" className="grid grid-cols-3 gap-2">
        {THEME_OPTIONS.map(({ value, label, icon: Icon }) => {
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
              <span className={cn('relative text-[13px]', active ? 'font-semibold text-app' : 'text-muted')}>{label}</span>
              {value === 'auto' && active && (
                <span
                  className={cn('relative h-1.5 w-1.5 rounded-full', effective === 'dark' ? 'bg-indigo-400' : 'bg-amber-500')}
                  aria-label={effective === 'dark' ? 'Oscuro activo' : 'Claro activo'}
                />
              )}
            </button>
          );
        })}
      </div>

      <p className="text-[13px] leading-snug text-muted">
        En modo Auto, la interfaz usa el tema claro entre el amanecer (06:50) y el atardecer (21:40).
      </p>

      {/* Preview en vivo de ambos temas */}
      <div className="flex flex-wrap gap-3">
        {THEME_PREVIEWS.map((t, i) => (
          <motion.div
            key={t.key}
            initial={{ opacity: 0, scale: 0.96 }}
            animate={{ opacity: 1, scale: 1 }}
            transition={{ duration: 0.5, delay: 0.1 + i * 0.1, ease: easeOutQuart }}
            className="w-[180px] rounded-2xl p-2"
            style={{ backgroundColor: t.bg, border: `1px solid ${t.border}` }}
            aria-label={`Vista previa del tema ${t.label}`}
          >
            <div className="flex h-[96px] flex-col justify-between rounded-xl p-3" style={{ backgroundColor: t.surface, border: `1px solid ${t.border}` }}>
              <div className="flex items-center justify-between">
                <span
                  className="flex h-6 w-6 items-center justify-center rounded-lg"
                  style={{ backgroundColor: `${t.accent}1F`, color: t.accent }}
                >
                  <Sun size={13} strokeWidth={2.4} />
                </span>
                <span className="text-[9px] font-medium uppercase tracking-[0.08em]" style={{ color: t.faint }}>
                  {t.label}
                </span>
              </div>
              <div>
                <p className="text-[9px] font-medium uppercase tracking-[0.08em]" style={{ color: t.faint }}>
                  Producción
                </p>
                <p className="font-display text-lg font-semibold leading-tight" style={{ color: t.text }}>
                  6,12 <span className="text-[0.6em] font-medium" style={{ color: t.faint }}>kW</span>
                </p>
              </div>
            </div>
          </motion.div>
        ))}
      </div>
    </div>
  );
}

// ── §2 Conexión con Home Assistant ───────────────────────────────────────────

function ConnectionSection() {
  const { connectionStatus, getLivePower } = useEnergyData();
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
        heliosToast(`Conectado a Home Assistant · ${data.station || 'estación activa'}`, { tone: 'success' });
      } else {
        heliosToast('El servidor no llega a Home Assistant; reintentando en segundo plano.', { tone: 'warning' });
      }
    } catch {
      heliosToast('Sin respuesta del servidor Helios.', { tone: 'warning' });
    } finally {
      setTesting(false);
    }
  };

  const logout = async () => {
    try {
      await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' });
    } finally {
      window.dispatchEvent(new Event('helios-unauthorized'));
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm leading-snug text-muted">
        Helios lee tu Home Assistant en red local desde el servidor de la app. Tus datos nunca salen de tu casa: solo esta web se
        expone, protegida con tu contraseña.
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
          {testing ? 'Probando…' : 'Probar conexión'}
        </button>
        <span className="inline-flex h-8 items-center gap-2 rounded-full border border-app bg-surface px-3 text-xs font-medium text-muted">
          <span className="relative flex h-2 w-2">
            {connected && (
              <span className="absolute inline-flex h-full w-full animate-ping-soft rounded-full bg-emerald-500 opacity-60" />
            )}
            <span className={cn('relative inline-flex h-2 w-2 rounded-full', connected ? 'bg-emerald-500' : 'bg-amber-500')} />
          </span>
          {connected ? `Conectado · ${live.station || 'Home Assistant'}` : 'Reconectando con Home Assistant…'}
        </span>
        <button
          onClick={logout}
          className="ml-auto inline-flex h-9 items-center gap-2 rounded-full border border-app bg-surface px-4 text-[13px] font-semibold text-muted transition-colors hover:text-app"
        >
          Cerrar sesión
        </button>
      </div>

      {/* Entidades leídas (colapsable) */}
      <Accordion type="single" collapsible>
        <AccordionItem value="entidades" className="rounded-xl border border-app px-3">
          <AccordionTrigger className="py-3 text-[13px] font-medium text-muted hover:no-underline">
            Ver entidades que se leen
          </AccordionTrigger>
          <AccordionContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-xs">Entidad</TableHead>
                  <TableHead className="text-xs">Descripción</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {ENTIDADES.map((e) => (
                  <TableRow key={e.entidad}>
                    <TableCell className="py-2 font-mono text-xs text-app">{e.entidad}</TableCell>
                    <TableCell className="py-2 text-xs text-muted">{e.descripcion}</TableCell>
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

// ── §3 Instalación ───────────────────────────────────────────────────────────

type Municipio = [string, string, number, number];

const normText = (s: string) =>
  s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase();

function LocationField() {
  const [settings, update] = useEnergySettings();
  const { sunriseMin, sunsetMin } = useEnergyData();
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
        Ubicación
      </Label>
      <div className="relative">
        <MapPin size={15} className="pointer-events-none absolute left-3 top-1/2 z-10 -translate-y-1/2 text-faint" />
        <Input
          id="install-location"
          value={query}
          placeholder="Empieza a escribir tu población…"
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
            aria-label="Quitar ubicación (usar la de Home Assistant)"
            title="Quitar ubicación (usar la de Home Assistant)"
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
          ? `Sol hoy en ${settings.location.split(' (')[0]}: ↑ ${fmtTime(sunriseMin)} · ↓ ${fmtTime(sunsetMin)}`
          : `Usando la ubicación de Home Assistant: ↑ ${fmtTime(sunriseMin)} · ↓ ${fmtTime(sunsetMin)}`}
      </p>
    </div>
  );
}

function InstallationSection() {
  const [settings, update] = useEnergySettings();

  const chips = [
    {
      key: 'solis',
      name: 'Solis S5-EH1P5K-L',
      lines: ['10 × 440 W · 4,4 kWp', 'Batería Soluna 5 kWh'],
      color: 'var(--c-solis)',
      icon: Zap,
      extraIcon: BatteryCharging,
      from: -24,
    },
    {
      key: 'fox',
      name: 'Fox H1-3.0-E',
      lines: ['6 × 450 W · 2,7 kWp'],
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
            Nombre de la instalación
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

// ── §4 Precios y ahorro ──────────────────────────────────────────────────────

function toEsDecimal(v: number, decimals = 2): string {
  return v.toFixed(decimals).replace('.', ',');
}

function parseEsDecimal(s: string): number | null {
  const n = Number(s.trim().replace(',', '.'));
  return Number.isFinite(n) && n >= 0 ? n : null;
}

function PricesSection() {
  const [settings, update] = useEnergySettings();
  const [priceImport, setPriceImport] = useState(() => toEsDecimal(settings.priceImport));
  const [priceExport, setPriceExport] = useState(() => toEsDecimal(settings.priceExport));
  const [co2, setCo2] = useState(() => toEsDecimal(settings.co2Factor));
  const [saved, setSaved] = useState(false);

  const save = () => {
    const pi = parseEsDecimal(priceImport);
    const pe = parseEsDecimal(priceExport);
    const fc = parseEsDecimal(co2);
    if (pi === null || pe === null || fc === null) {
      heliosToast('Revisa los valores: deben ser números positivos.', { tone: 'warning' });
      return;
    }
    update({ priceImport: pi, priceExport: pe, co2Factor: fc });
    setSaved(true);
    heliosToast('Preferencias guardadas', { tone: 'success' });
    window.setTimeout(() => setSaved(false), 1000);
  };

  const fields = [
    { id: 'precio-compra', label: 'Precio de compra', value: priceImport, set: setPriceImport, suffix: '€/kWh' },
    { id: 'precio-vertido', label: 'Compensación por vertido', value: priceExport, set: setPriceExport, suffix: '€/kWh' },
    { id: 'factor-co2', label: 'Factor CO₂', value: co2, set: setCo2, suffix: 'kg/kWh' },
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
        Estos valores solo afectan a las estimaciones de ahorro y CO₂ mostradas en la app.
      </p>

      <div>
        <button
          onClick={save}
          className="bg-brand-gradient inline-flex h-9 min-w-[120px] items-center justify-center gap-2 rounded-full px-6 text-[13px] font-semibold text-white shadow-md transition-transform hover:scale-[1.03] active:scale-95"
        >
          {saved ? (
            <motion.span initial={{ scale: 0.5 }} animate={{ scale: 1 }} transition={{ type: 'spring', stiffness: 500, damping: 20 }} className="flex">
              <Check size={16} strokeWidth={2.6} />
            </motion.span>
          ) : (
            'Guardar'
          )}
        </button>
      </div>
    </div>
  );
}

// ── §5 Datos ─────────────────────────────────────────────────────────────────

function DataSection() {
  const { liveTick } = useEnergyData();
  const [lastSyncAt, setLastSyncAt] = useState(() => Date.now());
  const [nowTs, setNowTs] = useState(() => Date.now());
  const [spinning, setSpinning] = useState(false);

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

  const refresh = async () => {
    setSpinning(true);
    try {
      const res = await fetch('/api/solar/history/refresh', { method: 'POST', credentials: 'same-origin' });
      const body = (await res.json().catch(() => null)) as { rows?: number } | null;
      if (res.ok) {
        heliosToast(`Histórico resincronizado · ${body?.rows ?? 0} días consolidados`, { tone: 'success' });
      } else {
        heliosToast('No se pudo resincronizar el histórico.', { tone: 'warning' });
      }
    } catch {
      heliosToast('Sin respuesta del servidor Helios.', { tone: 'warning' });
    } finally {
      setLastSyncAt(Date.now());
      window.setTimeout(() => setSpinning(false), 600);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <p className="text-sm font-medium text-app">Datos en vivo e histórico</p>
        <p className="mt-0.5 max-w-md text-[13px] leading-snug text-muted">
          Las potencias llegan en tiempo real desde Home Assistant. El histórico diario se consolida cada noche en el servidor de
          Helios para el mapa anual.
        </p>
      </div>

      <div className="flex items-center gap-2 border-t border-app pt-3 text-[13px] text-muted">
        <span>
          Última actualización en vivo · <span className="font-medium tabular-nums text-app">hace {secs} s</span>
        </span>
        <button
          onClick={refresh}
          aria-label="Resincronizar histórico ahora"
          className="ml-auto inline-flex items-center gap-2 rounded-full border border-app bg-surface px-3 py-2 text-muted transition-colors hover:text-app"
        >
          <motion.span animate={spinning ? { rotate: 360 } : { rotate: 0 }} transition={{ duration: 0.6 }} className="flex">
            <RefreshCw size={14} />
          </motion.span>
          <span className="text-xs font-semibold">Resincronizar histórico</span>
        </button>
      </div>
    </div>
  );
}

// ── §6 Instalar como app (PWA) ───────────────────────────────────────────────

function InstallSection() {
  const [deferredPrompt, setDeferredPrompt] = useState<BeforeInstallPromptEvent | null>(null);

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

  const steps = [
    'Abre esta página en el navegador de tu móvil',
    'Pulsa "Añadir a pantalla de inicio"',
    'Listo: acceso directo con icono propio',
  ];

  return (
    <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
      <motion.img
        src="/pwa-icon-512.png"
        alt="Icono de la app Helios"
        initial={{ scale: 0.8, opacity: 0 }}
        whileInView={{ scale: 1, opacity: 1 }}
        viewport={{ once: true }}
        transition={{ type: 'spring', stiffness: 260, damping: 18 }}
        className="h-24 w-24 shrink-0 rounded-[22px] shadow-lg"
      />
      <div className="min-w-0">
        <p className="font-display text-[15px] font-semibold text-app">Helios · Monitor Solar</p>
        <p className="mt-1 text-sm leading-snug text-muted">
          Instala la app en tu móvil u ordenador para ver tu energía a pantalla completa, como una app nativa.
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
        <div className="mt-4">
          {deferredPrompt ? (
            <button
              onClick={install}
              className="bg-brand-gradient inline-flex h-9 items-center gap-2 rounded-full px-5 text-[13px] font-semibold text-white shadow-md transition-transform hover:scale-[1.03] active:scale-95"
            >
              <Smartphone size={15} />
              Instalar
            </button>
          ) : (
            <p className="inline-flex items-center gap-2 rounded-full border border-app bg-surface-2 px-4 py-2 text-xs text-muted">
              <Smartphone size={14} className="text-faint" />
              Tu navegador ofrecerá la instalación desde su menú cuando esté disponible.
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
    for (const s of SECTIONS) {
      const el = sectionRefs.current[s.id];
      if (el) observer.observe(el);
    }
    return () => observer.disconnect();
  }, []);

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      {/* Encabezado */}
      <motion.header initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.4, ease: 'easeOut' }}>
        <h1 className="font-display text-2xl font-semibold tracking-[-0.01em] text-app">Ajustes</h1>
        <p className="text-sm text-muted">Configura la app, la conexión local y los precios de tu energía.</p>
      </motion.header>

      <div className="xl:flex xl:items-start xl:justify-center xl:gap-10">
        {/* Mini-nav anclada (solo ≥xl) */}
        <aside className="hidden w-[150px] shrink-0 xl:block">
          <nav aria-label="Secciones de ajustes" className="sticky top-24 flex flex-col gap-0.5">
            {SECTIONS.map((s) => {
              const active = activeSection === s.id;
              return (
                <button
                  key={s.id}
                  onClick={() => sectionRefs.current[s.id]?.scrollIntoView({ behavior: 'smooth', block: 'start' })}
                  aria-current={active}
                  className={cn(
                    'relative rounded-lg px-3 py-2 text-left text-[13px] font-medium transition-colors',
                    active ? 'bg-surface-2 text-amber-600 dark:text-amber-400' : 'text-faint hover:bg-surface-2/50 hover:text-muted',
                  )}
                >
                  {active && (
                    <motion.span layoutId="ajustes-nav-bar" className="bg-brand-gradient absolute left-0 top-2 h-5 w-[3px] rounded-full" />
                  )}
                  {s.label}
                </button>
              );
            })}
          </nav>
        </aside>

        {/* Columna de secciones */}
        <div className="mx-auto flex w-full max-w-[860px] flex-col gap-5 xl:mx-0">
          <Section id="tema" title="Apariencia" innerRef={(el) => (sectionRefs.current.tema = el)}>
            <ThemeSection />
          </Section>

          <Section id="conexion" title="Conexión local" innerRef={(el) => (sectionRefs.current.conexion = el)}>
            <ConnectionSection />
          </Section>

          <Section id="instalacion" title="Tu instalación" innerRef={(el) => (sectionRefs.current.instalacion = el)}>
            <InstallationSection />
          </Section>

          <Section id="precios" title="Precios de energía" innerRef={(el) => (sectionRefs.current.precios = el)}>
            <PricesSection />
          </Section>

          <Section id="datos" title="Datos" innerRef={(el) => (sectionRefs.current.datos = el)}>
            <DataSection />
          </Section>

          <Section id="app" title="Llévalo en tu móvil" innerRef={(el) => (sectionRefs.current.app = el)}>
            <InstallSection />
          </Section>

          {/* §7 Acerca de */}
          <motion.div
            initial={{ opacity: 0 }}
            whileInView={{ opacity: 1 }}
            viewport={{ once: true }}
            transition={{ duration: 0.6 }}
            className="flex flex-col items-center gap-1.5 py-6 text-center text-[13px] text-faint"
          >
            <img src="/logo.svg" alt="" className="h-7 w-7" />
            <p className="font-medium">Helios v0.1 · Monitor solar doméstico</p>
            <p>Datos locales vía Home Assistant · Sin nube, sin cuentas</p>
            <p>Hecho con ☀ en casa</p>
          </motion.div>
        </div>
      </div>

      <HeliosToaster />
    </div>
  );
}
