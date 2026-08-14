import { Routes, Route } from 'react-router';
import { ThemeProvider } from '@/theme/ThemeProvider';
import { EnergyDataProvider } from '@/data/EnergyDataProvider';
import AuthGate from '@/components/AuthGate';
import AppLayout from '@/components/shell/AppLayout';
import Dashboard from '@/pages/Dashboard';
import Inversores from '@/pages/Inversores';
import Bateria from '@/pages/Bateria';
import Historico from '@/pages/Historico';
import Ajustes from '@/pages/Ajustes';
import Cargador from '@/pages/Cargador';
import StubPage from '@/pages/StubPage';

export default function App() {
  return (
    <ThemeProvider>
      <AuthGate>
        <EnergyDataProvider>
          {/* Patrón children: AppLayout renderiza {children} envolviendo <Routes>. */}
          <AppLayout>
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/inversores" element={<Inversores />} />
              <Route path="/bateria" element={<Bateria />} />
              <Route path="/historico" element={<Historico />} />
              <Route path="/cargador" element={<Cargador />} />
              <Route path="/ajustes" element={<Ajustes />} />
              <Route path="*" element={<StubPage />} />
            </Routes>
          </AppLayout>
        </EnergyDataProvider>
      </AuthGate>
    </ThemeProvider>
  );
}
