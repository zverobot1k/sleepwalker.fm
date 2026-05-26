'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { setSession } from '@/lib/session';

export default function AuthCallbackPage() {
  const router = useRouter();

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const userId = params.get('user_id');
    if (!userId) {
      router.replace('/');
      return;
    }

    setSession({
      userId,
      displayName: params.get('display_name') || undefined,
      email: params.get('email') || undefined,
    });

    router.replace('/dashboard');
  }, [router]);

  return <div className="min-h-screen grid place-items-center text-muted-foreground">Signing you in...</div>;
}
