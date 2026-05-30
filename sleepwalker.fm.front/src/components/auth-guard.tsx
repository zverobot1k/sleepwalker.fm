'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useSession } from '@/hooks/use-session';

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const session = useSession();

  useEffect(() => {
    if (!session?.userId) {
      router.replace('/');
    }
  }, [router, session?.userId]);

  return <>{children}</>;
}
