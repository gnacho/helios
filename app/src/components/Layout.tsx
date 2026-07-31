import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { dateLocale } from '@/i18n';
import Navbar from '@/components/Navbar';
import Footer from '@/components/Footer';
import ThemeToggle from '@/components/ThemeToggle';
import ConnectionStatus from '@/components/ConnectionStatus';
import { useEnergyData } from '@/data/EnergyDataProvider';

/**
 * AppShell completo:
 *  - ≥lg: sidebar en flujo normal (flex) + columna de contenido.
 *  - <lg: header superior sticky (56px) + contenido con pb-24 + bottom nav fija.
 * El slot de contenido usa el patrón {children}: App.tsx envuelve <Routes>
 * dentro de <Layout>. No mezclar con <Outlet/>.
 */
export default function Layout({ children }: { children: ReactNode }) {
  const { today } = useEnergyData();
  const { t } = useTranslation();

  return (
    <div className="min-h-[100dvh] bg-app text-app">
      <div className="flex min-h-[100dvh]">
        <Navbar />

        <div className="flex min-w-0 flex-1 flex-col">
          {/* Header móvil (<lg), sticky en flujo normal */}
          <header className="sticky top-0 z-40 flex h-14 items-center justify-between border-b border-app bg-surface/85 px-4 backdrop-blur-[16px] lg:hidden">
            <Link to="/" aria-label={t('nav.dashboard')} className="flex items-center gap-2 rounded-lg transition-opacity hover:opacity-80">
              <img src="/logo.svg" alt="Helios" className="h-7 w-7" />
              <div className="leading-tight">
                <p className="font-display text-sm font-semibold text-app">Helios</p>
                <p className="text-[10px] font-medium capitalize text-faint">
                  {format(today, 'EEE d MMM', { locale: dateLocale() })}
                </p>
              </div>
            </Link>
            <div className="flex items-center gap-2">
              <ConnectionStatus compact />
              <ThemeToggle />
            </div>
          </header>

          {/* Contenido: pb-24 en móvil deja hueco al bottom nav fijo */}
          <main className="mx-auto w-full max-w-[1440px] flex-1 px-4 pb-24 pt-4 lg:px-6 lg:pb-6 lg:pt-6">
            {children}
          </main>

          <div className="hidden lg:block">
            <Footer />
          </div>
        </div>
      </div>
    </div>
  );
}
