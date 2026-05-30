'use client';

import { useEffect, useState } from 'react';
import { consumeOAuthSearchParams, getSession, Session } from '@/lib/session';
import { api, cacheSessionState, readCachedSessionState } from '@/lib/api';

function readSession(): Session | null {
  consumeOAuthSearchParams();
  return getSession();
}

export function useSession() {
  const [session, setSession] = useState<Session | null>(() =>
    typeof window !== 'undefined' ? readSession() : null,
  );

  useEffect(() => {
    const current = readSession();
    setSession(current);

    const userId = current?.userId;
    if (!userId) return;

    const cached = readCachedSessionState(userId);
    if (cached) {
      return;
    }

    let cancelled = false;
    api.session(userId)
      .then((state) => {
        if (cancelled) return;
        cacheSessionState(userId, state);
      })
      .catch(() => {
        // Session state is best-effort; UI can still continue with local session data.
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return session;
}
