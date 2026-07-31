import { Routes, Route } from 'react-router-dom';
import { ThemeProvider } from '@/theme/ThemeProvider';
import { EnergyDataProvider } from '@/data/EnergyDataProvider';
import AuthGate from '@/components/AuthGate';
import Layout from '@/components/Layout';
import Dashboard from '@/pages/Dashboard';
import Inversores from '@/pages/Inversores';
import Bateria from '@/pages/Bateria';
import Historico from '@/pages/Historico';
import Ajustes from '@/pages/Ajustes';
import StubPage from '@/pages/StubPage';

export default function App() {
  return (
    <ThemeProvider>
      <AuthGate>
        <EnergyDataProvider>
          {/* Patrón children: Layout renderiza {children} envolviendo <Routes>. */}
          <Layout>
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/inversores" element={<Inversores />} />
              <Route path="/bateria" element={<Bateria />} />
              <Route path="/historico" element={<Historico />} />
              <Route path="/ajustes" element={<Ajustes />} />
              <Route path="*" element={<StubPage />} />
            </Routes>
          </Layout>
        </EnergyDataProvider>
      </AuthGate>
    </ThemeProvider>
  );
}
