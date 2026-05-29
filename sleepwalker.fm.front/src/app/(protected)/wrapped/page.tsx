'use client';

import { useEffect, useState } from 'react';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { EmptyState, ErrorState, LoadingState } from '@/components/ui-state';
import { useI18n } from '@/components/providers/i18n-provider';
import { InfoBanner } from '@/components/info-banner';
import { resolveNotice } from '@/lib/notices';

export default function WrappedPage() {
  const session = useSession();
  const { t, lang } = useI18n();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [partialNotice, setPartialNotice] = useState(false);
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
      setPartialNotice(false);
      try {
        const results = await Promise.allSettled([
          api.wrappedSummary(session.userId),
          api.wrappedInsights(session.userId),
          api.wrappedTimeline(session.userId),
          api.wrappedCompare(session.userId),
        ]);
        if (!mounted) return;

        const anySuccess = results.some((result) => result.status === 'fulfilled');
        const anyFailure = results.some((result) => result.status === 'rejected');
        if (!anySuccess) {
          const first = results.find((r) => r.status === 'rejected');
          const message = first?.status === 'rejected' && first.reason instanceof Error
            ? first.reason.message
            : t('failedWrapped');
          setError(message);
        } else if (anyFailure) {
          setPartialNotice(true);
        }

        const valueOrNull = <T,>(result: PromiseSettledResult<T>) =>
          result.status === 'fulfilled' ? result.value : null;

        setSummary(valueOrNull(results[0]));
        setInsights(valueOrNull(results[1]));
        setTimeline(valueOrNull(results[2]));
        setCompare(valueOrNull(results[3]));
      } catch (e) {
        if (!mounted) return;
        setError(e instanceof Error ? e.message : t('failedWrapped'));
      } finally {
        if (mounted) setLoading(false);
      }
    })();

    return () => {
      mounted = false;
    };
  }, [session?.userId, t]);

  const notices = [
    partialNotice ? t('partialLoad') : null,
    resolveNotice(lang, summary?.notice, summary?.warning),
    resolveNotice(lang, insights?.notice, insights?.warning),
    resolveNotice(lang, compare?.notice, compare?.warning),
  ].filter((n): n is string => Boolean(n));

  if (!session?.userId || loading) return <LoadingState text={t('loading')} />;
  if (error) return <ErrorState message={error} />;

  return (
    <div className="space-y-6">
      <h1 className="text-4xl font-bold bg-gradient-to-r from-violet-200 to-indigo-200 bg-clip-text text-transparent">{t('wrapped')}</h1>

      {notices.length > 0 && (
        <div className="space-y-2">
          {[...new Set(notices)].map((text) => (
            <InfoBanner key={text} text={text} />
          ))}
        </div>
      )}

      <section className="p-5 rounded-2xl border border-glass-border bg-glass-bg">
        <h2 className="text-xl font-semibold mb-3">{t('highlights')}</h2>
        {!insights?.highlights?.length ? (
          <EmptyState text={t('empty')} />
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
          <h2 className="text-lg font-semibold mb-3">{t('topGenres')}</h2>
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
          <h2 className="text-lg font-semibold mb-3">{t('compare')}</h2>
          {!compare ? (
            <EmptyState text={t('empty')} />
          ) : (
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>{t('trackOverlap')}: {compare.track_overlap}</p>
              <p>{t('artistOverlap')}: {compare.artist_overlap}</p>
            </div>
          )}
        </div>
      </section>

      <section className="p-5 rounded-2xl border border-glass-border bg-glass-bg">
        <h2 className="text-lg font-semibold mb-3">{t('timeline')}</h2>
        {!timeline?.by_hour?.length ? (
          <EmptyState text={t('empty')} />
        ) : (
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
