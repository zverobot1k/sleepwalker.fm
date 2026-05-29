'use client';

import { useCallback, useEffect, useState } from 'react';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { EmptyState, ErrorState, LoadingState } from '@/components/ui-state';
import { useI18n } from '@/components/providers/i18n-provider';
import { InfoBanner } from '@/components/info-banner';
import { resolveNotice, resolveReason, resolveSourceLabel } from '@/lib/notices';

type Mode = 'comfort' | 'explore';

export default function RecommendationsPage() {
  const session = useSession();
  const { t, lang } = useI18n();
  const [mode, setMode] = useState<Mode>('comfort');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<Awaited<ReturnType<typeof api.recommendations>> | null>(null);

  const load = useCallback(async () => {
    if (!session?.userId) return;
    setLoading(true);
    setError(null);
    try {
      const response = await api.recommendations(session.userId, mode, 20);
      setData(response);
    } catch (e) {
      setError(e instanceof Error ? e.message : t('failedRecommendations'));
    } finally {
      setLoading(false);
    }
  }, [session?.userId, mode, t]);

  useEffect(() => {
    load();
  }, [load]);

  const notice = resolveNotice(lang, data?.notice, data?.warning);

  if (!session?.userId || loading) return <LoadingState text={t('loading')} />;
  if (error) return <ErrorState message={error} />;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h1 className="text-4xl font-bold bg-gradient-to-r from-violet-200 to-indigo-200 bg-clip-text text-transparent">
          {t('recommendations')}
        </h1>
        <div className="flex gap-2">
          {(['comfort', 'explore'] as Mode[]).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMode(m)}
              className={`px-4 py-2 rounded-lg text-sm border ${
                mode === m
                  ? 'border-violet-400 bg-violet-500/20 text-foreground'
                  : 'border-glass-border bg-glass-bg text-muted-foreground'
              }`}
            >
              {m === 'comfort' ? t('comfortMode') : t('exploreMode')}
            </button>
          ))}
        </div>
      </div>

      <p className="text-sm text-muted-foreground">
        {t('source')}: {resolveSourceLabel(lang, data?.source)}
      </p>
      {notice && <InfoBanner text={notice} />}

      {!data?.items?.length ? (
        <EmptyState text={t('empty')} />
      ) : (
        <div className="grid lg:grid-cols-2 gap-4">
          {data.items.map((item, idx) => (
            <article key={`${item.track?.id || idx}-${idx}`} className="p-4 rounded-xl border border-glass-border bg-glass-bg">
              <div className="flex items-center gap-3 mb-2">
                <span className="inline-flex h-7 w-7 items-center justify-center rounded-full border border-glass-border bg-secondary/30 text-xs font-semibold text-foreground">
                  {idx + 1}
                </span>
                <h2 className="font-semibold">{item.track?.name || '—'}</h2>
              </div>
              <p className="text-sm text-muted-foreground">{(item.track?.artists || []).map((a) => a.name).join(', ')}</p>
              <p className="text-xs mt-2 text-violet-200">{resolveReason(lang, item.reason)}</p>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}
