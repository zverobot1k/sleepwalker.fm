'use client';

import { useEffect } from 'react';
import { api } from '@/lib/api';

/** Proactively refreshes Spotify tokens when the server session is near expiry. */
export function useTokenRefresh(userId: string | undefined) {
  useEffect(() => {
    if (!userId) return;

    let cancelled = false;

    const tick = async () => {
      try {
        const state = await api.session(userId);
        if (cancelled || !state.connected) return;

        const expiresAt = new Date(state.expires_at).getTime();
        const fiveMinutes = 5 * 60 * 1000;
        if (Date.now() > expiresAt - fiveMinutes) {
          await api.refreshTokens(userId);
        }
      } catch {
        // Graceful degradation: pages surface API errors individually.
      }
    };

    tick();
    const id = window.setInterval(tick, 5 * 60 * 1000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [userId]);
}
