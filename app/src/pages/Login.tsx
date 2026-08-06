import { useState } from 'react';
import type { FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';
import { KeyRound, User } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import BrandLogo from '@/components/BrandLogo';
import i18n, { LANG_MODE_KEY } from '@/i18n';
import { ApiError, apiFetch, apiPost, resetAuthGuard } from '@/data/api-client';

/**
 * Fuerza el prompt "guardar contraseña" del navegador tras un login OK en SPA
 * (sin esto, al no haber recarga de página, el gestor no la ofrece).
 */
async function storeCredentials(username: string, password: string) {
  try {
    const PC = (
      window as unknown as {
        PasswordCredential?: new (d: { id: string; password: string; name?: string }) => Credential;
      }
    ).PasswordCredential;
    if ('credentials' in navigator && PC) {
      await navigator.credentials.store(new PC({ id: username, password, name: username }));
    }
  } catch {
    /* el usuario rechazó o el navegador no lo soporta: ignorar */
  }
}

export default function Login({ onSuccess }: { onSuccess: () => void }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();
  const navigate = useNavigate();

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await apiPost('/api/auth/login', { username, password });
      await storeCredentials(username, password);
      // Idioma del perfil: fuerza el idioma salvo en dispositivos en modo "auto".
      const me = await apiFetch<{ user?: { language?: string } }>('/api/auth/me').catch(() => null);
      const lang = me?.user?.language;
      if (lang && localStorage.getItem(LANG_MODE_KEY) !== 'auto') {
        if (i18n.language !== lang) await i18n.changeLanguage(lang);
        if (!localStorage.getItem(LANG_MODE_KEY)) localStorage.setItem(LANG_MODE_KEY, 'manual');
      }
      resetAuthGuard();
      navigate('/', { replace: true });
      onSuccess();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('login.error'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-dvh items-center justify-center bg-app p-4">
      <motion.div
        initial={{ opacity: 0, y: 24, scale: 0.97 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ duration: 0.45, ease: [0.25, 1, 0.5, 1] }}
        className="helios-card w-full max-w-sm p-7 shadow-card dark:shadow-card-dark"
      >
        <div className="mb-6 flex flex-col items-center gap-2 text-center">
          <BrandLogo className="h-16 w-16" />
          <h1 className="font-display text-xl font-semibold tracking-[-0.01em] text-app">{t('login.title')}</h1>
          <p className="text-sm text-muted">{t('login.subtitle')}</p>
        </div>

        <form onSubmit={submit} className="flex flex-col gap-4">
          <div>
            <Label htmlFor="login-user" className="mb-1.5 block text-[13px] font-medium text-muted">
              {t('login.username')}
            </Label>
            <div className="relative">
              <User size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint" />
              <Input
                id="login-user"
                autoComplete="username"
                autoFocus
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="pl-9"
              />
            </div>
          </div>
          <div>
            <Label htmlFor="login-pass" className="mb-1.5 block text-[13px] font-medium text-muted">
              {t('login.password')}
            </Label>
            <div className="relative">
              <KeyRound size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint" />
              <Input
                id="login-pass"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="pl-9"
              />
            </div>
          </div>

          {error && (
            <motion.p
              initial={{ opacity: 0, y: -4 }}
              animate={{ opacity: 1, y: 0 }}
              className="rounded-lg bg-rose-500/10 px-3 py-2 text-center text-[13px] font-medium text-rose-600 dark:text-rose-400"
              role="alert"
            >
              {error}
            </motion.p>
          )}

          <button
            type="submit"
            disabled={busy || !username || !password}
            className="bg-brand-gradient mt-1 inline-flex h-10 items-center justify-center rounded-full text-sm font-semibold text-white shadow-md transition-transform hover:scale-[1.02] active:scale-95 disabled:opacity-60 disabled:hover:scale-100"
          >
            {busy ? t('login.loading') : t('login.submit')}
          </button>
        </form>
      </motion.div>
    </div>
  );
}
