'use client';

import { useEffect, useState } from 'react';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { EmptyState, ErrorState, LoadingState, WarningState } from '@/components/ui-state';

export default function WrappedPage() {
  const session = useSession();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [summary, setSummary] = useState<Awaited<ReturnType<typeof api.wrappedSummary>> | null>(null);
  const [insights, setInsights] = useState<Awaited<ReturnType<typeof api.wrappedInsights>> | null>(null);
  const [timeline, setTimeline] = useState<Awaited<ReturnType<typeof api.wrappedTimeline>> | null>(null);
  const [compare, setCompare] = useState<Awaited<ReturnType<typeof api.wrappedCompare>> | null>(null);

  useEffect(() => {
    if (!session?.userId) return;
    let mounted = true;

    (async () => {
      setLoading(true);
      setError(null);
      setWarnings([]);
      try {
        const labels = ['Wrapped summary', 'Wrapped insights', 'Wrapped timeline', 'Wrapped compare'];
        const results = await Promise.allSettled([
          api.wrappedSummary(session.userId),
          api.wrappedInsights(session.userId),
          api.wrappedTimeline(session.userId),
          api.wrappedCompare(session.userId),
        ]);
        if (!mounted) return;

        const failures = results
          .map((result, idx) => {
            if (result.status === 'fulfilled') return null;
            const message = result.reason instanceof Error ? result.reason.message : 'Request failed';
            return `${labels[idx]}: ${message}`;
          })
          .filter((value): value is string => Boolean(value));

        const anySuccess = results.some((result) => result.status === 'fulfilled');
        if (!anySuccess) {
          setError(failures[0] || 'Failed to load wrapped');
        } else {
          setWarnings(failures);
        }

        const valueOrNull = <T,>(result: PromiseSettledResult<T>) =>
          result.status === 'fulfilled' ? result.value : null;

        setSummary(valueOrNull(results[0]));
        setInsights(valueOrNull(results[1]));
        setTimeline(valueOrNull(results[2]));
        setCompare(valueOrNull(results[3]));
      } catch (e) {
        if (!mounted) return;
        setError(e instanceof Error ? e.message : 'Failed to load wrapped');
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
      <h1 className="text-4xl font-bold bg-gradient-to-r from-violet-200 to-indigo-200 bg-clip-text text-transparent">Wrapped</h1>

      {warnings.length > 0 && (
        <div className="space-y-2">
          {warnings.map((warning, idx) => (
            <WarningState key={`${warning}-${idx}`} text={warning} />
          ))}
        </div>
      )}
      {summary?.warning && <WarningState text={summary.warning} />}
      {insights?.warning && <WarningState text={insights.warning} />}

      <section className="p-5 rounded-2xl border border-glass-border bg-glass-bg">
        <h2 className="text-xl font-semibold mb-3">Highlights</h2>
        {!insights?.highlights?.length ? (
          <EmptyState />
        ) : (
          <ul className="space-y-2 list-disc pl-6">
            {insights.highlights.map((h, idx) => (
              <li key={idx}>{h}</li>
            ))}
          </ul>
        )}
      </section>

      <section className="grid md:grid-cols-2 gap-6">
        <div className="p-5 rounded-2xl border border-glass-border bg-glass-bg">
          <h2 className="text-lg font-semibold mb-3">Top Genres</h2>
          <ul className="space-y-2 text-sm">
            {(summary?.top_genres || []).slice(0, 10).map((g, index) => (
              <li key={g.genre} className="flex items-center justify-between gap-3">
                <span className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-glass-border bg-secondary/30 text-xs font-semibold text-foreground">
                  {index + 1}
                </span>
                <span className="flex-1">{g.genre}</span>
                <span>{g.count}</span>
              </li>
            ))}
          </ul>
        </div>

        <div className="p-5 rounded-2xl border border-glass-border bg-glass-bg">
          <h2 className="text-lg font-semibold mb-3">Compare</h2>
          {!compare ? (
            <EmptyState />
          ) : (
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>Track overlap: {compare.track_overlap}</p>
              <p>Artist overlap: {compare.artist_overlap}</p>
              {compare.warning && <p className="text-amber-200">{compare.warning}</p>}
            </div>
          )}
        </div>
      </section>

      <section className="p-5 rounded-2xl border border-glass-border bg-glass-bg">
        <h2 className="text-lg font-semibold mb-3">Timeline</h2>
        {!timeline?.by_hour?.length ? <EmptyState /> : (
          <div className="grid grid-cols-2 md:grid-cols-6 gap-2 text-xs text-muted-foreground">
            {timeline.by_hour.map((item) => (
              <div key={item.key} className="rounded-lg p-2 border border-glass-border bg-secondary/20">
                <div>{item.key}</div>
                <div className="font-semibold text-foreground">{item.plays}</div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
