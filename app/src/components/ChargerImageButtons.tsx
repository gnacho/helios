import { useRef, useState } from 'react';
import { ImagePlus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { ChargerCfg } from '@/data/types';
import { apiFetch } from '@/data/api-client';
import { heliosToast } from '@/lib/toast';

/** Boton pequeno (arriba-derecha de la tarjeta) para cambiar la foto del
 *  coche: reescala a 460 px de alto en canvas (conserva transparencia) y la
 *  sube al endpoint del cargador. La imagen vive en dataDir, no en el repo. */
export default function ChargerImageButtons({ charger, onUploaded }: { charger: ChargerCfg; onUploaded?: () => void }) {
  const { t } = useTranslation();
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);

  const onFile = async (file: File) => {
    if (busy) return;
    setBusy(true);
    try {
      // data: URL de partida (CSP: img-src permite data:, blob: NO)
      const original: string = await new Promise((res, rej) => {
        const fr = new FileReader();
        fr.onload = () => res(String(fr.result));
        fr.onerror = rej;
        fr.readAsDataURL(file);
      });
      const img = new Image();
      await new Promise((res, rej) => { img.onload = () => res(null); img.onerror = () => rej(new Error(`load fail len=${original.length} w=${img.naturalWidth} h=${img.naturalHeight} head=${original.slice(0, 30)}`)); img.src = original; });
      const scale = Math.min(1, 460 / img.naturalHeight);
      const w = Math.max(1, Math.round(img.naturalWidth * scale));
      const h = Math.max(1, Math.round(img.naturalHeight * scale));
      const canvas = document.createElement('canvas');
      canvas.width = w;
      canvas.height = h;
      canvas.getContext('2d')?.drawImage(img, 0, 0, w, h);
      const dataUrl = canvas.toDataURL('image/png');
      await apiFetch(`/api/charger/image/${charger.key}`, {
        method: 'PUT',
        body: JSON.stringify({ image: dataUrl }),
        headers: { 'Content-Type': 'application/json' },
      });
      onUploaded?.();
      heliosToast(t('cargador.imageOk'));
    } catch (err) {
      console.error('[imagen]', err);
      const detalle = err instanceof Error && err.message ? err.message : String(err);
      heliosToast(detalle || t('common.error'), { tone: 'warning' });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="ml-auto flex items-center gap-1.5">
      <input
        ref={fileRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void onFile(f);
          e.target.value = '';
        }}
      />
      <button
        type="button"
        onClick={() => fileRef.current?.click()}
        disabled={busy}
        title={t('cargador.imageEdit')}
        aria-label={t('cargador.imageEdit')}
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-app bg-surface text-muted transition-colors hover:bg-surface-2 hover:text-app disabled:opacity-50"
      >
        <ImagePlus size={15} strokeWidth={1.75} />
      </button>
    </div>
  );
}
