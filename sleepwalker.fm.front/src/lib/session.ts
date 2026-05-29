export type Session = {
  userId: string;
  displayName?: string;
  email?: string;
};

const KEY = 'swfm_session';

export function getSession(): Session | null {
  if (typeof window === 'undefined') return null;
  const raw = localStorage.getItem(KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as Session;
  } catch {
    return null;
  }
}

export function setSession(session: Session) {
  if (typeof window === 'undefined') return;
  localStorage.setItem(KEY, JSON.stringify(session));
}

export function clearSession() {
  if (typeof window === 'undefined') return;
  localStorage.removeItem(KEY);
}

/** Apply one-time OAuth query params from backend redirect, then strip them from the URL. */
export function consumeOAuthSearchParams(): boolean {
  if (typeof window === 'undefined') return false;
  const params = new URLSearchParams(window.location.search);
  const userId = params.get('user_id');
  if (!userId) return false;

  setSession({
    userId,
    displayName: params.get('display_name') || undefined,
    email: params.get('email') || undefined,
  });
  window.history.replaceState({}, '', window.location.pathname);
  return true;
}
