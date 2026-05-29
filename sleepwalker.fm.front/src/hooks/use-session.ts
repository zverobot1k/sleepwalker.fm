'use client';

import { useEffect, useState } from 'react';
import { consumeOAuthSearchParams, getSession, Session } from '@/lib/session';

function readSession(): Session | null {
  consumeOAuthSearchParams();
  return getSession();
}

export function useSession() {
  const [session, setSession] = useState<Session | null>(() =>
    typeof window !== 'undefined' ? readSession() : null,
  );

  useEffect(() => {
    setSession(readSession());
  }, []);

  return session;
}
