import { Moon, Sun } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@/theme/ThemeProvider';

/**
 * Toggle binario Sol/Luna para el header móvil (issue #95): alterna claro/oscuro
 * según el tema efectivo. El selector de 3 estados (auto/claro/oscuro) queda en
 * la topbar desktop, donde las etiquetas son visibles.
 */
export default function ThemeToggleButton() {
  const { effective, setMode } = useTheme();
  const { t } = useTranslation();
  const dark = effective === 'dark';
  const label = dark ? t('theme.toLight') : t('theme.toDark');

  return (
    <button
      type="button"
      onClick={() => setMode(dark ? 'light' : 'dark')}
      aria-label={label}
      title={label}
      className="rounded-full border border-app bg-surface p-2 text-muted transition-colors hover:text-app"
    >
      {dark ? <Sun size={16} /> : <Moon size={16} />}
    </button>
  );
}
