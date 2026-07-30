import { useState } from 'react';
import { Link, NavLink, useLocation } from 'react-router-dom';
import { motion } from 'framer-motion';
import {
  Activity,
  Zap,
  BatteryCharging,
  BarChart3,
  Settings,
  ChevronsLeft,
  ChevronsRight,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import ConnectionStatus from '@/components/ConnectionStatus';

export const NAV_ITEMS = [
  { to: '/', label: 'Hoy', icon: Activity },
  { to: '/inversores', label: 'Inversores', icon: Zap },
  { to: '/bateria', label: 'Batería', icon: BatteryCharging },
  { to: '/historico', label: 'Histórico', icon: BarChart3 },
  { to: '/ajustes', label: 'Ajustes', icon: Settings },
] as const;

function isActive(pathname: string, to: string): boolean {
  if (to === '/') return pathname === '/';
  return pathname.startsWith(to);
}

/**
 * Navegación principal:
 *  - ≥lg: sidebar izquierdo en flujo normal (232px, colapsable a riel de 72px).
 *  - <lg: bottom navigation fija (64px + safe-area).
 */
export default function Navbar() {
  const [collapsed, setCollapsed] = useState(false);
  const { pathname } = useLocation();

  return (
    <>
      {/* ── Sidebar ≥ lg ─────────────────────────────────────────── */}
      <aside
        className={cn(
          'sticky top-0 hidden h-[100dvh] shrink-0 flex-col border-r border-app bg-surface lg:flex',
          'transition-[width] duration-300 ease-out',
          collapsed ? 'w-[72px]' : 'w-[232px]',
        )}
      >
        {/* Logo */}
        <Link
          to="/"
          aria-label="Ir a Hoy"
          className={cn('flex items-center gap-2.5 rounded-xl px-4 pb-6 pt-5 transition-opacity hover:opacity-80', collapsed && 'justify-center px-0')}
        >
          <img src="/logo.svg" alt="Helios" className="h-9 w-9 shrink-0" />
          {!collapsed && (
            <div className="min-w-0">
              <p className="font-display text-lg font-semibold leading-tight tracking-tight text-app">Helios</p>
              <p className="truncate text-[11px] font-medium text-faint">Monitor Solar</p>
            </div>
          )}
        </Link>

        {/* Nav vertical */}
        <nav className="flex flex-1 flex-col gap-1 px-3" aria-label="Navegación principal">
          {NAV_ITEMS.map(({ to, label, icon: Icon }) => {
            const active = isActive(pathname, to);
            return (
              <NavLink
                key={to}
                to={to}
                aria-label={label}
                className={cn(
                  'group relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors',
                  collapsed && 'justify-center px-0',
                  active ? 'bg-surface-2 text-amber-500' : 'text-muted hover:bg-surface-2/50 hover:text-app',
                )}
              >
                {active && (
                  <motion.span
                    layoutId="nav-indicator"
                    className="bg-brand-gradient absolute left-0 top-1/2 h-6 w-[3px] -translate-y-1/2 rounded-full"
                  />
                )}
                <Icon
                  size={20}
                  strokeWidth={active ? 2.4 : 2}
                  className="shrink-0 transition-transform duration-200 group-hover:translate-x-0.5"
                />
                {!collapsed && <span>{label}</span>}
              </NavLink>
            );
          })}
        </nav>

        {/* Pie: conexión + colapsar */}
        <div className={cn('flex flex-col gap-2 border-t border-app p-3', collapsed && 'items-center')}>
          {collapsed ? (
            <ConnectionStatus compact />
          ) : (
            <div className="flex items-center justify-between gap-2">
              <ConnectionStatus />
              <button
                onClick={() => setCollapsed(true)}
                aria-label="Colapsar menú"
                className="rounded-lg p-1.5 text-faint transition-colors hover:bg-surface-2 hover:text-muted"
              >
                <ChevronsLeft size={18} />
              </button>
            </div>
          )}
          {collapsed && (
            <button
              onClick={() => setCollapsed(false)}
              aria-label="Expandir menú"
              className="rounded-lg p-1.5 text-faint transition-colors hover:bg-surface-2 hover:text-muted"
            >
              <ChevronsRight size={18} />
            </button>
          )}
        </div>
      </aside>

      {/* ── Bottom navigation < lg ───────────────────────────────── */}
      <nav
        className="fixed bottom-0 left-0 right-0 z-50 border-t border-app bg-surface/85 pb-safe backdrop-blur-[16px] lg:hidden"
        aria-label="Navegación principal"
      >
        <div className="grid h-16 grid-cols-5">
          {NAV_ITEMS.map(({ to, label, icon: Icon }) => {
            const active = isActive(pathname, to);
            return (
              <NavLink
                key={to}
                to={to}
                className="relative flex flex-col items-center justify-center gap-1"
                aria-label={label}
              >
                <motion.span
                  animate={active ? { scale: [1, 1.15, 1] } : { scale: 1 }}
                  transition={{ duration: 0.25, type: 'spring', stiffness: 500, damping: 20 }}
                  className={cn('flex', active ? 'text-amber-500' : 'text-faint')}
                >
                  <Icon size={22} strokeWidth={active ? 2.4 : 2} />
                </motion.span>
                <span className={cn('text-[10px] font-medium', active ? 'text-amber-500' : 'text-faint')}>
                  {label}
                </span>
                {active && <span className="absolute bottom-1.5 h-1 w-1 rounded-full bg-amber-500" />}
              </NavLink>
            );
          })}
        </div>
      </nav>
    </>
  );
}
