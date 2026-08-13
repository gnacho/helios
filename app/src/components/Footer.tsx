import { useTranslation } from 'react-i18next';
import { useInstall } from '@/hooks/useInstall';
import { useAppVersion } from '@/hooks/useAppVersion';
import { numLocale } from '@/i18n';

export default function Footer() {
  const { t } = useTranslation();
  const install = useInstall();
  const version = useAppVersion();

  // Resumen de la instalación desde la topología (issue #37): sin nombres de
  // marca hardcodeados. "N inversores · Batería X kWh" (batería opcional).
  const installLine = buildInstallLine(install);

  return (
    <footer className="mx-auto w-full max-w-[1440px] px-4 pb-6 pt-2 lg:px-6">
      <div className="flex flex-col items-center justify-between gap-2 border-t border-app pt-4 text-xs text-faint sm:flex-row">
        <p>{t('footer.line1', { version })}</p>
        <p>{installLine ?? t('footer.line2')}</p>
      </div>
    </footer>
  );
}

function buildInstallLine(install: ReturnType<typeof useInstall>): string | null {
  if (!install) return null;
  const invs = install.inverters;
  if (invs.length === 0) return null;
  const nf = new Intl.NumberFormat(numLocale(), { maximumFractionDigits: 1 });
  const invPart = invs.length === 1 ? '1 inversor' : `${invs.length} inversores`;
  const batPart = install.battery.enabled && install.battery.capacityKwh > 0
    ? ` · Batería ${nf.format(install.battery.capacityKwh)} kWh`
    : '';
  return `${invPart}${batPart}`;
}
