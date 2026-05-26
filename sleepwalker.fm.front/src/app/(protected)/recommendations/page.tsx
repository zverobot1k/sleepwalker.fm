'use client';

import { useEffect, useState } from 'react';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { EmptyState, ErrorState, LoadingState, WarningState } from '@/components/ui-state';

export default function RecommendationsPage() {
  const session = useSession();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<Awaited<ReturnType<typeof api.recommendations>> | null>(null);

  useEffect(() => {
    if (!session?.userId) return;
    let mounted = true;

    (async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await api.recommendations(session.userId, 'comfort', 20);
        if (!mounted) return;
        setData(response);
      } catch (e) {
        if (!mounted) return;
        setError(e instanceof Error ? e.message : 'Failed to load recommendations');
      } finally {
        if (mounted) setLoading(false);
      }
    })();

    return () => {
      mounted = false;
    };
  }, [session?.userId]);

  if (!session?.userId || loading) return <LoadingState />;
  if (error) return <ErrorState message={error} />;

  return (
    <div className="space-y-6">
      <h1 className="text-4xl font-bold bg-gradient-to-r from-violet-200 to-indigo-200 bg-clip-text text-transparent">Recommendations</h1>
      <p className="text-sm text-muted-foreground">Source: {data?.source || '-'}</p>
      {data?.warning && <WarningState text={data.warning} />}

      {!data?.items?.length ? (
        <EmptyState />
      ) : (
        <div className="grid lg:grid-cols-2 gap-4">
          {data.items.map((item, idx) => (
            <article key={`${item.track?.id || idx}-${idx}`} className="p-4 rounded-xl border border-glass-border bg-glass-bg">
              <h2 className="font-semibold">{item.track?.name || '-'}</h2>
              <p className="text-sm text-muted-foreground">{(item.track?.artists || []).map((a) => a.name).join(', ')}</p>
              <p className="text-xs mt-2 text-violet-200">{item.reason}</p>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}
