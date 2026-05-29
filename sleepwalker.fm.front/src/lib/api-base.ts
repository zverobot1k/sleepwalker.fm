declare global {
  interface Window {
    /** Injected at runtime from server env API_URL (see root layout). */
    __SWFM_API_BASE__?: string;
  }
}

let loggedBase = false;

function isBrowser(): boolean {
  return typeof window !== 'undefined';
}

function rejectUnsafeBrowserBase(base: string): void {
  if (!isBrowser()) return;
  let host = '';
  try {
    host = new URL(base).hostname.toLowerCase();
  } catch {
    throw new Error(`Invalid API base URL: ${base}`);
  }
  if (host === 'backend' || base.includes('backend:8080')) {
    throw new Error('API base URL must not use Docker internal host "backend:8080" in the browser');
  }
}

/** Resolve API origin: runtime injection → build-time env. Never use docker-internal hosts in browser. */
export function getApiBase(): string {
  const raw =
    (isBrowser() ? window.__SWFM_API_BASE__ : undefined) ||
    process.env.API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    process.env.NEXT_PUBLIC_API_BASE_URL ||
    '';

  const base = raw.replace(/\/+$/, '');
  if (!base) {
    throw new Error('API base URL is not configured (set API_URL or NEXT_PUBLIC_API_URL)');
  }

  rejectUnsafeBrowserBase(base);

  if (isBrowser() && !loggedBase) {
    loggedBase = true;
    console.info('[sleepwalker.fm] API base URL:', base);
  }

  return base;
}

export function isNgrokHost(base: string): boolean {
  try {
    const host = new URL(base).hostname.toLowerCase();
    return host.includes('ngrok') || host.endsWith('.ngrok-free.dev') || host.endsWith('.ngrok.io');
  } catch {
    return false;
  }
}

/** Headers required for browser → ngrok API requests (skip interstitial). */
export function apiRequestHeaders(init?: HeadersInit): Headers {
  const headers = new Headers(init);
  const base = getApiBase();
  if (isNgrokHost(base)) {
    headers.set('ngrok-skip-browser-warning', '1');
  }
  return headers;
}

export function describeFetchError(url: string, err: unknown): string {
  const reason = err instanceof Error ? err.message : String(err);
  if (/failed to fetch|load failed|networkerror/i.test(reason)) {
    return `Network error calling ${url} (${reason}). Check API URL, CORS, and ngrok tunnel.`;
  }
  return reason;
}
