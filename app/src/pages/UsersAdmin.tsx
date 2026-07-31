import { useState } from 'react';
import type { FormEvent } from 'react';
import { motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';
import { KeyRound, Plus, User, UserPlus } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { heliosToast } from '@/lib/toast';

export default function UsersAdmin() {
  const { t } = useTranslation();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [language, setLanguage] = useState('es');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const languages = [
    { code: 'es', name: 'Español', flag: '🇪🇸' },
    { code: 'en', name: 'English', flag: '🇬🇧' },
    { code: 'zh-CN', name: '简体中文', flag: '🇨🇳' },
  ];

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch('/api/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ username, password, language }),
      });
      const body = (await res.json()) as { error?: string; ok?: boolean; user?: unknown };
      if (!res.ok) {
        setError(body?.error ?? t('common.error'));
        return;
      }
      heliosToast(`Usuario ${username} creado correctamente`, { tone: 'success' });
      setUsername('');
      setPassword('');
      setLanguage('es');
    } catch {
      setError(t('common.error'));
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
        className="helios-card w-full max-w-md p-7 shadow-card dark:shadow-card-dark"
      >
        <div className="mb-6 flex flex-col items-center gap-2 text-center">
          <span className="bg-brand-gradient flex h-14 w-14 items-center justify-center rounded-2xl shadow-lg">
            <UserPlus size={28} className="text-white" strokeWidth={2.2} />
          </span>
          <h1 className="font-display text-xl font-semibold tracking-[-0.01em] text-app">{t('admin.users.title')}</h1>
          <p className="text-sm text-muted">{t('admin.users.subtitle')}</p>
        </div>

        <form onSubmit={submit} className="flex flex-col gap-4">
          <div>
            <Label htmlFor="admin-user" className="mb-1.5 block text-[13px] font-medium text-muted">
              {t('admin.users.username')}
            </Label>
            <div className="relative">
              <User size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint" />
              <Input
                id="admin-user"
                autoComplete="username"
                autoFocus
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="pl-9"
              />
            </div>
          </div>

          <div>
            <Label htmlFor="admin-pass" className="mb-1.5 block text-[13px] font-medium text-muted">
              {t('admin.users.password')}
            </Label>
            <div className="relative">
              <KeyRound size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint" />
              <Input
                id="admin-pass"
                type="password"
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="pl-9"
              />
            </div>
          </div>

          <div>
            <Label htmlFor="admin-lang" className="mb-1.5 block text-[13px] font-medium text-muted">
              {t('admin.users.language')}
            </Label>
            <Select value={language} onValueChange={setLanguage}>
              <SelectTrigger id="admin-lang" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {languages.map((lang) => (
                  <SelectItem key={lang.code} value={lang.code}>
                    <span className="flex items-center gap-2">
                      <span>{lang.flag}</span>
                      <span>{lang.name}</span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
            className="bg-brand-gradient mt-1 inline-flex h-10 items-center justify-center gap-2 rounded-full text-sm font-semibold text-white shadow-md transition-transform hover:scale-[1.02] active:scale-95 disabled:opacity-60 disabled:hover:scale-100"
          >
            <Plus size={16} />
            {busy ? t('admin.users.creating') : t('admin.users.create')}
          </button>
        </form>
      </motion.div>
    </div>
  );
}
