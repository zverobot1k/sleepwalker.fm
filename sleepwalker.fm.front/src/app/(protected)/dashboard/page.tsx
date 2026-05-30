'use client';

import { useEffect, useMemo, useState } from 'react';
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, PieChart, Pie, Cell, RadarChart, Radar, PolarGrid, PolarAngleAxis } from 'recharts';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { EmptyState, ErrorState, LoadingState } from '@/components/ui-state';
import { useI18n } from '@/components/providers/i18n-provider';
import { InfoBanner } from '@/components/info-banner';
import { formatGenreDisplay, resolveNotice, resolveReason, resolveSourceLabel } from '@/lib/notices';

type DashboardData = {
  snapshot: Awaited<ReturnType<typeof api.snapshot>> | null;
  recs: Awaited<ReturnType<typeof api.recommendations>> | null;
};

const colors = ['#8b5cf6', '#6366f1', '#a78bfa', '#c4b5fd', '#7c3aed', '#4f46e5'];

export default function DashboardPage() {
  const session = useSession();
  const { t, lang } = useI18n();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [partialNotice, setPartialNotice] = useState(false);
  const [data, setData] = useState<DashboardData>({
    snapshot: null,
    recs: null,
  });

  const spotifySnapshot = data.snapshot?.snapshot || null;
  const topGenres = data.snapshot?.top_genres || [];
  const recentPlays = spotifySnapshot?.recently_played || [];

  const timelineData = useMemo(() => {
    return buildDayTimeline(recentPlays).slice(-7);
  }, [recentPlays]);

  useEffect(() => {
    if (!session?.userId) return;
    let mounted = true;

    (async () => {
      setLoading(true);
      setError(null);
      setPartialNotice(false);
      try {
        const results = await Promise.allSettled([
          api.snapshot(session.userId),
          api.recommendations(session.userId, 'comfort', 20),
        ]);

        if (!mounted) return;

        const anySuccess = results.some((result) => result.status === 'fulfilled');
        const anyFailure = results.some((result) => result.status === 'rejected');
        if (!anySuccess) {
          const first = results.find((r) => r.status === 'rejected');
          const message = first?.status === 'rejected' && first.reason instanceof Error
            ? first.reason.message
            : 'Failed to load dashboard';
          setError(message);
        } else if (anyFailure) {
          setPartialNotice(true);
        }

        const valueOrNull = <T,>(result: PromiseSettledResult<T>) =>
          result.status === 'fulfilled' ? result.value : null;

        setData({
          snapshot: valueOrNull(results[0]),
          recs: valueOrNull(results[1]),
        });
      } catch (e) {
        if (!mounted) return;
        setError(e instanceof Error ? e.message : 'Failed to load dashboard');
      } finally {
        if (mounted) setLoading(false);
      }
    })();

    return () => {
      mounted = false;
    };
  }, [session?.userId]);

  const displayedGenres = useMemo(
    () =>
      topGenres
        .map((entry) => ({ ...entry, genre: formatGenreDisplay(lang, entry.genre) }))
        .filter((entry) => entry.genre),
    [topGenres, lang],
  );

  const listeningStats = useMemo(() => computeListeningStats(recentPlays), [recentPlays]);

  const avgAudio = useMemo(() => {
    const items = spotifySnapshot?.audio_features || [];
        useEffect(() => {
          if (!session?.userId) return;
          let mounted = true;

          (async () => {
            setLoading(true);
            setError(null);
            setPartialNotice(false);

            try {
              // PHASE 1: Fetch snapshot first (should be fast, <100ms)
              let snapshotData = null;
              try {
                snapshotData = await api.snapshot(session.userId);
                if (mounted) {
                  setData((prev) => ({
                    ...prev,
                    snapshot: snapshotData,
                  }));
                  setLoading(false); // UI renders immediately with snapshot
                }
              } catch (snapshotError) {
                if (mounted) {
                  setError(
                    snapshotError instanceof Error
                      ? snapshotError.message
                      : 'Failed to load snapshot'
                  );
                  setLoading(false);
                }
                return; // Stop if snapshot fails
              }

              // PHASE 2: Fetch recommendations async (non-blocking, background)
              try {
                const recsData = await api.recommendations(session.userId, 'comfort', 20);
                if (mounted) {
                  setData((prev) => ({
                    ...prev,
                    recs: recsData,
                  }));
                }
              } catch (recsError) {
                // Recommendations failure is not critical, just set partial notice
                if (mounted) {
                  setPartialNotice(true);
                  console.warn('Recommendations load failed:', recsError);
                }
              }
            } catch (e) {
              if (mounted) {
                setError(e instanceof Error ? e.message : 'Failed to load dashboard');
                setLoading(false);
              }
            }
          })();

          return () => {
            mounted = false;
          };
        }, [session?.userId]);
      )}

      <div className="grid lg:grid-cols-2 gap-6">
        <Panel title={t('genreDistribution')}>
          {!displayedGenres.length ? (
            <EmptyState text={t('empty')} />
          ) : (
            <div className="h-72">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie data={displayedGenres.slice(0, 8)} dataKey="count" nameKey="genre" outerRadius={100}>
                    {displayedGenres.slice(0, 8).map((_, i) => (
                      <Cell key={i} fill={colors[i % colors.length]} />
                    ))}
                  </Pie>
                  <Tooltip />
                </PieChart>
              </ResponsiveContainer>
            </div>
          )}
        </Panel>

        <Panel title={t('listeningTimeline')}>
          {!timelineData.length ? (
            <EmptyState text={t('empty')} />
          ) : (
            <div className="h-72">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={timelineData}>
                  <XAxis dataKey="key" stroke="#c4b5fd" />
                  <YAxis stroke="#c4b5fd" />
                  <Tooltip />
                  <Bar dataKey="plays" fill="#8b5cf6" radius={[6, 6, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          )}
        </Panel>

        <Panel title={t('audioFeatures')}>
          {!avgAudio ? (
            <EmptyState text={t('empty')} />
          ) : (
            <div className="h-72">
              <ResponsiveContainer width="100%" height="100%">
                <RadarChart data={avgAudio}>
                  <PolarGrid stroke="#4c1d95" />
                  <PolarAngleAxis dataKey="metric" stroke="#ddd6fe" />
                  <Radar dataKey="value" stroke="#8b5cf6" fill="#8b5cf6" fillOpacity={0.35} />
                  <Tooltip />
                </RadarChart>
              </ResponsiveContainer>
            </div>
          )}
        </Panel>

        <Panel title={t('wrappedSummary')}>
          {!spotifySnapshot ? (
            <EmptyState text={t('empty')} />
          ) : (
            <div className="space-y-3 text-sm text-muted-foreground">
              <p>
                {t('topArtistsLabel')}: {(spotifySnapshot.top_artists || []).slice(0, 3).map((a) => a.name).join(', ') || '—'}
              </p>
              <p>
                {t('topTracksLabel')}: {(spotifySnapshot.top_tracks || []).slice(0, 3).map((tr) => tr.name).join(', ') || '—'}
              </p>
              <p>
                {t('topGenresLastFm')}: {displayedGenres.slice(0, 3).map((entry) => entry.genre).join(', ') || '—'}
              </p>
              <p>
                {t('recentPlays')}: {listeningStats.plays ?? '—'}
              </p>
            </div>
          )}
        </Panel>

        <Panel title={t('recommendationsPanel')}>
          {!data.recs?.items?.length ? (
            <EmptyState text={t('empty')} />
          ) : (
            <ul className="space-y-2 text-sm">
              {data.recs.items.slice(0, 5).map((item, idx) => (
                <li key={`${item.track?.id || idx}`}>
                  <span className="font-medium">{item.track?.name}</span>
                  <span className="text-muted-foreground text-xs block">{resolveReason(lang, item.reason)}</span>
                </li>
              ))}
              <li className="text-xs text-muted-foreground pt-1">
                {t('source')}: {resolveSourceLabel(lang, data.recs.source)}
              </li>
            </ul>
          )}
        </Panel>
      </div>

      <div className="grid lg:grid-cols-3 gap-6">
        <Panel title={t('topArtists')}>
          <ul className="space-y-2 text-sm">
            {(spotifySnapshot?.top_artists || []).slice(0, 8).map((a, index) => (
              <li key={a.id} className="flex items-center justify-between gap-3">
                <span className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-glass-border bg-secondary/30 text-xs font-semibold text-foreground">
                  {index + 1}
                </span>
                <span className="flex-1">{a.name}</span>
                <span className="text-muted-foreground text-xs">
                  {a.score != null ? `${t('score')} ${a.score.toFixed(1)}` : a.popularity ?? '—'}
                </span>
              </li>
            ))}
          </ul>
        </Panel>

        <Panel title={t('topTracks')}>
          <ul className="space-y-2 text-sm">
            {(spotifySnapshot?.top_tracks || []).slice(0, 8).map((track, index) => (
              <li key={track.id} className="space-y-1">
                <div className="flex items-center gap-3">
                  <span className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-glass-border bg-secondary/30 text-xs font-semibold text-foreground">
                    {index + 1}
                  </span>
                  <span className="flex-1">{track.name}</span>
                </div>
                <div className="text-muted-foreground text-xs">{(track.artists || []).map((a) => a.name).join(', ')}</div>
              </li>
            ))}
          </ul>
        </Panel>

        <Panel title={t('recentlyPlayed')}>
          <ul className="space-y-2 text-sm">
            {recentPlays.slice(0, 8).map((r, idx) => (
              <li key={`${r.track?.id || idx}-${idx}`} className="space-y-1">
                <div className="flex items-center gap-3">
                  <span className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-glass-border bg-secondary/30 text-xs font-semibold text-foreground">
                    {idx + 1}
                  </span>
                  <span className="flex-1">{r.track?.name || '—'}</span>
                </div>
                <div className="text-muted-foreground text-xs">{new Date(r.played_at).toLocaleString()}</div>
              </li>
            ))}
          </ul>
        </Panel>
      </div>
    </div>
  );
}

function computeListeningStats(
  items: Array<{
    track?: {
      id?: string;
      duration_ms?: number;
      artists?: Array<{ id?: string }>;
    };
    played_at: string;
  }>,
) {
  const uniqueTracks = new Set<string>();
  const uniqueArtists = new Set<string>();
  let durationMs = 0;

  for (const item of items) {
    durationMs += item.track?.duration_ms || 0;
    if (item.track?.id) {
      uniqueTracks.add(item.track.id);
    }
    for (const artist of item.track?.artists || []) {
      if (artist?.id) {
        uniqueArtists.add(artist.id);
      }
    }
  }

  return {
    plays: items.length,
    durationMs,
    minutes: durationMs / 60000,
    hours: durationMs / 3600000,
    uniqueTracks: uniqueTracks.size,
    uniqueArtists: uniqueArtists.size,
  };
}

function buildDayTimeline(items: Array<{ played_at: string }>) {
  const counts = new Map<string, number>();
  for (const item of items) {
    const date = new Date(item.played_at);
    if (Number.isNaN(date.getTime())) continue;
    const key = date.toISOString().slice(0, 10);
    counts.set(key, (counts.get(key) || 0) + 1);
  }

  return Array.from(counts.entries())
    .map(([key, plays]) => ({ key, plays }))
    .sort((a, b) => a.key.localeCompare(b.key));
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="p-5 rounded-2xl border border-glass-border bg-glass-bg backdrop-blur-xl">
      <h2 className="text-lg font-semibold mb-4">{title}</h2>
      {children}
    </section>
  );
}

function Metric({ title, value }: { title: string; value: string }) {
  return (
    <div className="p-4 rounded-xl border border-glass-border bg-glass-bg backdrop-blur-xl">
      <p className="text-xs text-muted-foreground">{title}</p>
      <p className="text-2xl font-semibold mt-1">{value}</p>
    </div>
  );
}
