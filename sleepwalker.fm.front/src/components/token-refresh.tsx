'use client';

import { useSession } from '@/hooks/use-session';
import { useTokenRefresh } from '@/hooks/use-token-refresh';

export function TokenRefresh() {
  const session = useSession();
  useTokenRefresh(session?.userId);
  return null;
}
