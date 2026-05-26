'use client';

import { useEffect, useState } from 'react';
import { getSession, Session } from '@/lib/session';

export function useSession() {
  const [session, setSession] = useState<Session | null>(null);

  useEffect(() => {
    setSession(getSession());
  }, []);

  return session;
}
