import { motion } from 'framer-motion';

interface StubPageProps {
  title: string;
  description: string;
}

/** Página provisional mientras se implementa la vista definitiva. */
export default function StubPage({ title, description }: StubPageProps) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, ease: 'easeOut' }}
      className="flex flex-col gap-4"
    >
      <header>
        <h1 className="font-display text-2xl font-semibold tracking-[-0.01em] text-app">{title}</h1>
        <p className="text-sm text-muted">{description}</p>
      </header>
      <div className="helios-card flex flex-col items-center gap-4 p-10 text-center shadow-card dark:shadow-card-dark">
        <img src="/empty-solar.svg" alt="" className="w-full max-w-[320px]" />
        <p className="text-sm font-medium text-muted">Esta vista está en construcción.</p>
        <p className="max-w-sm text-xs text-faint">
          Los datos de esta sección se servirán desde la misma capa mock del dashboard (EnergyDataProvider), lista para el
          conector HAOS de la fase 2.
        </p>
      </div>
    </motion.div>
  );
}
