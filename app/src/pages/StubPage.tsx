import { motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';

/** Página provisional mientras se implementa la vista definitiva. */
export default function StubPage() {
  const { t } = useTranslation();
  return (
    <motion.div
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, ease: 'easeOut' }}
      className="flex flex-col gap-4"
    >
      <header>
        <h1 className="font-display text-2xl font-semibold tracking-[-0.01em] text-app">{t('stub.title')}</h1>
        <p className="text-sm text-muted">{t('stub.desc')}</p>
      </header>
      <div className="helios-card flex flex-col items-center gap-4 p-10 text-center shadow-card dark:shadow-card-dark">
        <img src="/empty-solar.svg" alt="" className="w-full max-w-[320px]" />
        <p className="text-sm font-medium text-muted">{t('stub.body')}</p>
      </div>
    </motion.div>
  );
}
