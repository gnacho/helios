/**
 * Registro del sistema de toasts de Helios. `HeliosToaster` (componente)
 * registra aquí su función push al montarse; cualquier módulo puede lanzar
 * un toast con `heliosToast(...)` sin acoplarse al árbol de React.
 */

export type ToastTone = 'success' | 'warning' | 'info';

export type ToastPushFn = (message: string, opts?: { tone?: ToastTone }) => void;

let pushRef: ToastPushFn | null = null;

export function registerToastPush(fn: ToastPushFn | null) {
  pushRef = fn;
}

export function heliosToast(message: string, opts?: { tone?: ToastTone }) {
  pushRef?.(message, opts);
}
