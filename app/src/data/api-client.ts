const APP_SLUG = 'helios';

export class ApiError extends Error {
  readonly status: number;
  readonly body?: unknown;

  constructor(message: string, status: number, body?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
  }
}

let handling401 = false;

export function resetAuthGuard(): void {
  handling401 = false;
}

function handleUnauthorized(): never {
  if (!handling401) {
    handling401 = true;
    window.dispatchEvent(new Event(`${APP_SLUG}-unauthorized`));
  }
  throw new ApiError('Sesión expirada', 401);
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: { Accept: 'application/json', ...init?.headers },
  });

  if (res.status === 401) handleUnauthorized();

  const contentType = res.headers.get('content-type') ?? '';
  if (!contentType.includes('application/json')) {
    throw new ApiError(`Respuesta no JSON (${res.status})`, res.status);
  }

  const body = (await res.json()) as unknown;

  if (!res.ok) {
    const message =
      typeof body === 'object' && body !== null && 'error' in body
        ? String((body as { error: unknown }).error)
        : `Error HTTP ${res.status}`;
    throw new ApiError(message, res.status, body);
  }

  return body as T;
}

export function apiPost<T>(path: string, data?: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: data === undefined ? undefined : JSON.stringify(data),
  });
}

export function apiPut<T>(path: string, data: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
}

export function apiDelete<T>(path: string): Promise<T> {
  return apiFetch<T>(path, { method: 'DELETE' });
}
