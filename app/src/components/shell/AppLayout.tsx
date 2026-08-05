import { useEffect, useState, type ReactNode } from 'react';
import { Link, NavLink, useLocation } from 'react-router-dom';
import { motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';
import {
  Activity,
  Zap,
  BatteryCharging,
  BarChart3,
  Settings,
  ChevronsLeft,
  ChevronsRight,
  RefreshCw,
  Sunrise,
  Sun,
  Moon,
  User,
  Wifi,
  WifiOff,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useTheme } from '@/theme/ThemeProvider';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { fmtWeekdayDate } from '@/i18n';
import BrandLogo from '@/components/BrandLogo';
import ConnectionStatus from '@/components/ConnectionStatus';
import AlertsBell from '@/components/AlertsBell';
import ThemeToggle from '@/components/ThemeToggle';
import Footer from '@/components/Footer';
import HeliosToaster from '@/components/HeliosToaster';
import { apiFetch } from '@/data/api-client';

/**
 * AppLayout unificado (skill webapp-shell):
 *  - ≥lg: sidebar 232px colapsable a raíl 64px (persiste en helios-sidebar-collapsed)
 *  - md: raíl 64px con tooltips
 *  - <md: header móvil + bottom nav (5 items)
 *  - Topbar desktop/tablet: título+fecha | refresh, campana, tema, conexión
 *  - DemoBanner cuando sessionStorage['helios-demo'] === '1'
 *  - HeliosToaster global (antes solo en Ajustes e Histórico)
 */

const NAV_ITEMS = [
  { to: '/', labelKey: 'nav.dashboard', icon: Activity, end: true },
  { to: '/inversores', labelKey: 'nav.inversores', icon: Zap },
  { to: '/bateria', labelKey: 'nav.bateria', icon: BatteryCharging },
  { to: '/historico', labelKey: 'nav.historico', icon: BarChart3 },
] as const;

const SETTINGS_ITEM = { to: '/ajustes', labelKey: 'nav.ajustes', icon: Settings } as const;
const ALL_ITEMS = [...NAV_ITEMS, SETTINGS_ITEM];

const TITLE_KEYS: [RegExp, string][] = [
  [/^\/$/, 'nav.dashboard'],
  [/^\/inversores/, 'nav.inversores'],
  [/^\/bateria/, 'nav.bateria'],
  [/^\/historico/, 'nav.historico'],
  [/^\/ajustes/, 'nav.ajustes'],
];

const ACTIVE = 'bg-surface-2 text-brand';
const IDLE = 'text-muted hover:bg-surface-2/50 hover:text-app';

function isActive(pathname: string, to: string): boolean {
  if (to === '/') return pathname === '/';
  return pathname.startsWith(to);
}

function Logo({ collapsed }: { collapsed?: boolean }) {
  const { t } = useTranslation();
  return (
    <Link
      to="/"
      aria-label={t('nav.dashboard')}
      className={cn(
        'flex h-16 shrink-0 items-center gap-2.5 rounded-xl px-4 transition-opacity hover:opacity-80',
        collapsed && 'justify-center px-0',
      )}
    >
      <BrandLogo className="h-9 w-9 shrink-0" />
      {!collapsed && (
        <div className="min-w-0">
          <p className="font-display text-lg font-semibold leading-tight tracking-tight text-app">Helios</p>
          <p className="truncate text-[11px] font-medium text-faint">{t('common.tagline')}</p>
        </div>
      )}
    </Link>
  );
}

/** Botón de tema compacto (cicla auto → claro → oscuro) para raíl/colapsado. */
function ThemeCycleButton() {
  const { mode, setMode } = useTheme();
  const { t } = useTranslation();
  const next = mode === 'auto' ? 'light' : mode === 'light' ? 'dark' : 'auto';
  const Icon = mode === 'auto' ? Sunrise : mode === 'light' ? Sun : Moon;
  return (
    <button
      type="button"
      onClick={() => setMode(next)}
      aria-label={t('theme.label')}
      title={t('theme.label')}
      className="flex h-9 w-9 items-center justify-center rounded-lg border border-app bg-surface text-muted transition-colors hover:text-app"
    >
      <Icon size={16} strokeWidth={1.75} />
    </button>
  );
}

/** Item de navegación solo-icono con tooltip (raíl md y sidebar colapsado). */
function IconNavLink({ to, labelKey, icon: Icon }: { to: string; labelKey: string; icon: typeof Activity }) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const active = isActive(pathname, to);
  return (
    <NavLink
      to={to}
      aria-label={t(labelKey)}
      className={cn(
        'group relative flex h-10 w-10 items-center justify-center rounded-lg transition-colors',
        active ? ACTIVE : IDLE,
      )}
    >
      <Icon size={18} strokeWidth={1.75} />
      <span className="pointer-events-none absolute left-full z-50 ml-2 hidden whitespace-nowrap rounded-md border border-app bg-surface px-2 py-1 text-xs text-app group-hover:block">
        {t(labelKey)}
      </span>
    </NavLink>
  );
}

function UserBlock({ collapsed }: { collapsed: boolean }) {
  const { t } = useTranslation();
  const { connectionStatus } = useEnergyData();
  const [username, setUsername] = useState<string | null>(null);
  const connected = connectionStatus === 'connected';

  useEffect(() => {
    apiFetch<{ authenticated?: boolean; user?: { username?: string } }>('/api/auth/me')
      .then((d) => { if (d.authenticated && d.user?.username) setUsername(d.user.username); })
      .catch(() => setUsername(null));
  }, []);

  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-1.5">
        <span className="flex h-8 w-8 items-center justify-center rounded-full bg-brand/12 text-brand">
          <User size={16} />
        </span>
        <span className={cn('h-2 w-2 rounded-full', connected ? 'bg-emerald-500' : 'bg-amber-500')} title={connected ? t('common.online') : t('ajustes.connection.reconnecting')} />
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2.5 rounded-xl border border-app bg-surface-2/50 px-3 py-2">
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-brand/12 text-brand">
        <User size={16} />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-[13px] font-semibold text-app">{username ?? '...'}</p>
        <span className="inline-flex items-center gap-1 text-[11px] text-faint">
          {connected
            ? <><Wifi size={10} /> {t('common.online')}</>
            : <><WifiOff size={10} /> {t('ajustes.connection.reconnecting')}</>
          }
        </span>
      </div>
    </div>
  );
}

function Sidebar({ collapsed, onToggleCollapse }: { collapsed: boolean; onToggleCollapse: () => void }) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const settingsActive = isActive(pathname, SETTINGS_ITEM.to);

  if (collapsed) {
    return (
      <aside className="fixed inset-y-0 left-0 z-40 hidden w-16 flex-col items-center border-r border-app bg-surface py-3 lg:flex">
        <Logo collapsed />
        <nav className="mt-4 flex flex-1 flex-col items-center gap-1" aria-label={t('nav.dashboard')}>
          {NAV_ITEMS.map((item) => (
            <IconNavLink key={item.to} {...item} />
          ))}
        </nav>
        <div className="flex flex-col items-center gap-2">
          <UserBlock collapsed />
          <div className="flex items-center gap-1.5">
            <ThemeCycleButton />
            <NavLink
              to={SETTINGS_ITEM.to}
              aria-label={t('nav.ajustes')}
              className={cn(
                'flex h-9 w-9 items-center justify-center rounded-lg transition-colors',
                settingsActive ? 'bg-brand/15 text-brand' : 'border border-app bg-surface text-muted hover:text-app',
              )}
            >
              <Settings size={16} strokeWidth={1.75} />
            </NavLink>
          </div>
          <button
            type="button"
            onClick={onToggleCollapse}
            aria-label={t('nav.expand')}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-app bg-surface text-muted transition-colors hover:text-app"
          >
            <ChevronsRight size={16} strokeWidth={1.75} />
          </button>
        </div>
      </aside>
    );
  }

  return (
    <aside className="fixed inset-y-0 left-0 z-40 hidden w-[232px] flex-col border-r border-app bg-surface lg:flex">
      <Logo />
      <nav className="flex-1 space-y-1 overflow-y-auto p-3" aria-label={t('nav.dashboard')}>
        {NAV_ITEMS.map(({ to, labelKey, icon: Icon }) => {
          const active = isActive(pathname, to);
          return (
            <NavLink
              key={to}
              to={to}
              className={cn(
                'group relative flex h-10 items-center gap-3 rounded-lg px-3 text-sm font-medium transition-colors duration-150',
                active ? ACTIVE : IDLE,
              )}
            >
              <Icon size={18} strokeWidth={1.75} className="shrink-0" />
              <span className="flex-1">{t(labelKey)}</span>
              {active && (
                <motion.span
                  layoutId="nav-indicator"
                  className="bg-brand-gradient absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-full"
                />
              )}
            </NavLink>
          );
        })}
      </nav>
      <div className="border-t border-app p-3">
        <UserBlock collapsed={false} />
        <div className="mt-2 flex items-center gap-2">
          <ThemeCycleButton />
          <NavLink
            to={SETTINGS_ITEM.to}
            className={cn(
              'flex h-9 flex-1 items-center gap-2 rounded-lg px-3 text-sm font-semibold transition-colors',
              settingsActive
                ? 'bg-brand/15 text-brand'
                : 'bg-brand/8 text-brand hover:bg-brand/15',
            )}
          >
            <Settings size={18} strokeWidth={1.75} />
            {t('nav.ajustes')}
          </NavLink>
          <button
            type="button"
            onClick={onToggleCollapse}
            aria-label={t('nav.collapse')}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-app bg-surface text-muted transition-colors hover:text-app"
          >
            <ChevronsLeft size={16} strokeWidth={1.75} />
          </button>
        </div>
      </div>
    </aside>
  );
}

function Rail() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const settingsActive = isActive(pathname, SETTINGS_ITEM.to);
  return (
    <aside className="fixed inset-y-0 left-0 z-40 hidden w-16 flex-col items-center border-r border-app bg-surface py-3 md:flex lg:hidden">
      <Logo collapsed />
      <nav className="mt-4 flex flex-1 flex-col items-center gap-1" aria-label={t('nav.dashboard')}>
        {ALL_ITEMS.map((item) => (
          <IconNavLink key={item.to} {...item} />
        ))}
      </nav>
      <div className="flex flex-col items-center gap-2">
        <UserBlock collapsed />
        <ThemeCycleButton />
        <NavLink
          to={SETTINGS_ITEM.to}
          aria-label={t('nav.ajustes')}
          className={cn(
            'flex h-9 w-9 items-center justify-center rounded-lg transition-colors',
            settingsActive ? 'bg-brand/15 text-brand' : 'border border-app bg-surface text-muted hover:text-app',
          )}
        >
          <Settings size={16} strokeWidth={1.75} />
        </NavLink>
      </div>
    </aside>
  );
}

/** Barra de modo demo (patrón zfsctl): visible mientras la sesión es demo. */
function DemoBanner({ onExit }: { onExit: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      role="status"
      className="mb-4 flex items-center gap-2.5 rounded-xl border border-amber-500/35 bg-amber-500/10 px-3.5 py-2.5 text-[13px] font-semibold text-amber-500"
    >
      <span className="h-2 w-2 shrink-0 rounded-full bg-amber-500 animate-ping-soft" />
      <span>{t('demo.banner')}</span>
      <button
        type="button"
        onClick={onExit}
        className="ml-auto flex h-8 items-center rounded-lg border border-amber-500/40 px-3 text-xs font-medium text-amber-500 transition-colors hover:bg-amber-500/15"
      >
        {t('demo.exit')}
      </button>
    </div>
  );
}

export default function AppLayout({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { refresh, today } = useEnergyData();
  const [spinning, setSpinning] = useState(false);
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem('helios-sidebar-collapsed') === '1';
    } catch {
      return false;
    }
  });
  const [isDemo, setIsDemo] = useState(() => {
    try {
      return sessionStorage.getItem('helios-demo') === '1';
    } catch {
      return false;
    }
  });

  const toggleCollapse = () => {
    setCollapsed((prev) => {
      try {
        localStorage.setItem('helios-sidebar-collapsed', prev ? '0' : '1');
      } catch {
        /* sin localStorage */
      }
      return !prev;
    });
  };

  const exitDemo = () => {
    try {
      sessionStorage.removeItem('helios-demo');
    } catch {
      /* sin sessionStorage */
    }
    setIsDemo(false);
    // Helios no tiene ruta /login: AuthGate escucha este evento y muestra Login.
    window.dispatchEvent(new Event('helios-unauthorized'));
    window.location.assign('/');
  };

  const onRefresh = () => {
    refresh();
    setSpinning(true);
    window.setTimeout(() => setSpinning(false), 600);
  };

  const titleKey = TITLE_KEYS.find(([re]) => re.test(pathname))?.[1] ?? 'nav.dashboard';
  const lgMargin = collapsed ? 'lg:ml-16' : 'lg:ml-[232px]';

  return (
    <div className="min-h-[100dvh] bg-app text-app">
      <Sidebar collapsed={collapsed} onToggleCollapse={toggleCollapse} />
      <Rail />

      {/* Topbar desktop/tablet */}
      <header
        className={cn(
          'sticky top-0 z-30 hidden h-14 items-center justify-between border-b border-app bg-app/80 px-6 backdrop-blur-[16px] md:flex md:ml-16',
          lgMargin,
        )}
      >
        <div className="min-w-0">
          <h1 className="font-display text-lg font-semibold leading-tight tracking-[-0.01em] text-app">{t(titleKey)}</h1>
          <p className="text-[11px] capitalize leading-tight text-faint">{fmtWeekdayDate(today)}</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            aria-label={t('common.refresh')}
            onClick={onRefresh}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-app bg-surface text-muted transition-colors hover:text-app"
          >
            <motion.span
              animate={spinning ? { rotate: 360 } : { rotate: 0 }}
              transition={{ duration: 0.6 }}
              className="flex"
            >
              <RefreshCw size={16} strokeWidth={1.75} />
            </motion.span>
          </button>
          <AlertsBell />
          <ThemeToggle />
          <ConnectionStatus />
        </div>
      </header>

      {/* Header móvil */}
      <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-app bg-app/85 px-4 backdrop-blur-[16px] md:hidden">
        <Link to="/" aria-label={t('nav.dashboard')} className="flex items-center gap-2">
          <BrandLogo className="h-8 w-8" />
          <span className="font-display text-base font-semibold text-app">Helios</span>
        </Link>
        <div className="flex items-center gap-2">
          <ConnectionStatus compact />
          <AlertsBell />
        </div>
      </header>

      <main className={cn('mx-auto max-w-[1440px] px-4 pb-24 pt-4 md:ml-16 md:px-6 md:pb-6 md:pt-6', lgMargin)}>
        {isDemo && <DemoBanner onExit={exitDemo} />}
        {children}
        <div className="hidden lg:block">
          <Footer />
        </div>
      </main>

      {/* Bottom nav < md */}
      <nav
        className="fixed bottom-0 left-0 right-0 z-50 border-t border-app bg-surface/85 pb-safe backdrop-blur-[16px] md:hidden"
        aria-label={t('nav.dashboard')}
      >
        <div className="grid h-16 grid-cols-5">
          {ALL_ITEMS.map(({ to, labelKey, icon: Icon }) => {
            const active = isActive(pathname, to);
            return (
              <NavLink key={to} to={to} className="relative flex flex-col items-center justify-center gap-1" aria-label={t(labelKey)}>
                <motion.span
                  animate={active ? { scale: [1, 1.15, 1] } : { scale: 1 }}
                  transition={{ duration: 0.25, type: 'spring', stiffness: 500, damping: 20 }}
                  className={cn('flex', active ? 'text-brand' : 'text-faint')}
                >
                  <Icon size={22} strokeWidth={active ? 2.4 : 2} />
                </motion.span>
                <span className={cn('text-[10px] font-medium', active ? 'text-brand' : 'text-faint')}>{t(labelKey)}</span>
                {active && <span className="absolute bottom-1.5 h-1 w-1 rounded-full bg-brand" />}
              </NavLink>
            );
          })}
        </div>
      </nav>

      <HeliosToaster />
    </div>
  );
}
