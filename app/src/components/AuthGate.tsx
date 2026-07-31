import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { Sun } from 'lucide-react';
import Login from '@/pages/Login';
import i18n, { LANG_MODE_KEY } from '@/i18n';

type AuthState = 'loading' | 'authed' | 'login';

/** Aplica el idioma del perfil (BD) salvo que este dispositivo esté en modo "auto". */
function applyProfileLanguage(user: { language?: string } | undefined) {
  if (!user?.language) return;
  const mode = localStorage.getItem(LANG_MODE_KEY);
  if (mode === 'auto') return; // este dispositivo sigue al navegador
  if (i18n.language !== user.language) void i18n.changeLanguage(user.language);
  if (!mode) localStorage.setItem(LANG_MODE_KEY, 'manual');
}

export default function AuthGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>('loading');

  useEffect(() => {
    let cancelled = false;
    fetch('/api/auth/me', { credentials: 'same-origin' })
      .then(async (res) => {
        if (cancelled) return;
        if (res.ok) {
          const data = (await res.json().catch(() => null)) as { user?: { language?: string } } | null;
          applyProfileLanguage(data?.user);
          setState('authed');
        } else {
          setState('login');
        }
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
