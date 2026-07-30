import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { Sun } from 'lucide-react';
import Login from '@/pages/Login';

type AuthState = 'loading' | 'authed' | 'login';

export default function AuthGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>('loading');

  useEffect(() => {
    let cancelled = false;
    fetch('/api/auth/me', { credentials: 'same-origin' })
      .then((res) => {
        if (cancelled) return;
        setState(res.ok ? 'authed' : 'login');
      })
      .catch(() => {
        if (!cancelled) setState('login');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const toLogin = () => setState('login');
    window.addEventListener('helios-unauthorized', toLogin);
    return () => window.removeEventListener('helios-unauthorized', toLogin);
  }, []);

  if (state === 'loading') {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-app">
        <span className="bg-brand-gradient flex h-14 w-14 animate-pulse items-center justify-center rounded-2xl shadow-lg">
          <Sun size={28} className="text-white" strokeWidth={2.2} />
        </span>
      </div>
    );
  }

  if (state === 'login') {
    return <Login onSuccess={() => setState('authed')} />;
  }

  return <>{children}</>;
}
