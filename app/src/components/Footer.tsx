import { useTranslation } from 'react-i18next';

export default function Footer() {
  const { t } = useTranslation();
  return (
    <footer className="mx-auto w-full max-w-[1440px] px-4 pb-6 pt-2 lg:px-6">
      <div className="flex flex-col items-center justify-between gap-2 border-t border-app pt-4 text-xs text-faint sm:flex-row">
        <p>{t('footer.line1')}</p>
        <p>{t('footer.line2')}</p>
      </div>
    </footer>
  );
}
