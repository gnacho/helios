import { useTheme } from '@/theme/ThemeProvider';

/** Logo de marca dual: versión clara en tema claro, oscura en tema oscuro. */
export default function BrandLogo({ className }: { className?: string }) {
  const { isDark } = useTheme();
  return (
    <img
      src={isDark ? '/icons/oscuro/helios-icon-192.png' : '/icons/claro/helios-icon-192.png'}
      alt="Helios"
      className={className}
    />
  );
}
