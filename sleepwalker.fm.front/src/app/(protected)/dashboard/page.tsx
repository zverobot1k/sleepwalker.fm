'use client';

import { useEffect, useMemo, useState } from 'react';
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, PieChart, Pie, Cell, RadarChart, Radar, PolarGrid, PolarAngleAxis } from 'recharts';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { EmptyState, ErrorState, LoadingState } from '@/components/ui-state';
import { useI18n } from '@/components/providers/i18n-provider';
import { InfoBanner } from '@/components/info-banner';
import { formatGenreDisplay, resolveReason, resolveSourceLabel } from '@/lib/notices';

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
  const [data, setData] = useState<DashboardData>({ snapshot: null, recs: null });

  const spotifySnapshot = data.snapshot?.snapshot || null;
  const topGenres = data.snapshot?.top_genres || [];
  const recentPlays = spotifySnapshot?.recently_played || [];

  useEffect(() => {
    if (!session?.userId) return;
    let mounted = true;

    (async () => {
      setLoading(true);
      setError(null);
      setPartialNotice(false);

      try {
        // Phase 1: snapshot — fast when Redis-cached (~50ms)
        try {
          const snapshotData = await api.snapshot(session.userId);
          if (mounted) {
            setData((prev) => ({ ...prev, snapshot: snapshotData }));
            setLoading(false);
          }
        } catch (snapshotError) {
          if (mounted) {
            setError(snapshotError instanceof Error ? snapshotError.message : 'Failed to load snapshot');
            setLoading(false);
          }
          return;
        }

        // Phase 2: recommendations in background — slower (Last.fm calls)
        try {
          const recsData = await api.recommendations(session.userId, 20);
          if (mounted) setData((prev) => ({ ...prev, recs: recsData }));
        } catch {
          if (mounted) setPartialNotice(true);
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
    if (!items.length) return null;
    const n = items.length;
    const sums = items.reduce(
      (acc, f) => ({
        danceability: acc.danceability + f.danceability,
        energy: acc.energy + f.energy,
        valence: acc.valence + f.valence,
        acousticness: acc.acousticness + (f.acousticness ?? 0),
      }),
      { danceability: 0, energy: 0, valence: 0, acousticness: 0 },
    );
    return [
      { metric: 'Danceability', value: +(sums.danceability / n).toFixed(2) },
      { metric: 'Energy', value: +(sums.energy / n).toFixed(2) },
      { metric: 'Valence', value: +(sums.valence / n).toFixed(2) },
      { metric: 'Acousticness', value: +(sums.acousticness / n).toFixed(2) },
    ];
  }, [spotifySnapshot]);

  const timelineData = useMemo(() => buildDayTimeline(recentPlays).slice(-7), [recentPlays]);

  if (!session?.userId || loading) return <LoadingState text={t('loading')} />;
  if (error) return <ErrorState message={error} />;

  return (
    <div className="space-y-6">
      <h1 className="text-4xl font-bold bg-gradient-to-r from-violet-200 to-indigo-200 bg-clip-text text-transparent">
        {t('dashboard')}
      </h1>

      {partialNotice && <InfoBanner text={t('partialLoad')} />}

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Metric title={t('recentPlays')} value={String(listeningStats.plays ?? '—')} />
        <Metric title={t('uniqueTracks')} value={String(listeningStats.uniqueTracks ?? '—')} />
        <Metric title={t('uniqueArtists')} value={String(listeningStats.uniqueArtists ?? '—')} />
        <Metric title={t('listeningHours')} value={listeningStats.hours ? listeningStats.hours.toFixed(1) + 'h' : '—'} />
      </div>

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
    if (item.track?.id) uniqueTracks.add(item.track.id);
    for (const artist of item.track?.artists || []) {
      if (artist?.id) uniqueArtists.add(artist.id);
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
