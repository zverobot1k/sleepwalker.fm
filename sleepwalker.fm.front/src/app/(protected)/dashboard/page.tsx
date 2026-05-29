'use client';

import { useEffect, useMemo, useState } from 'react';
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, PieChart, Pie, Cell, RadarChart, Radar, PolarGrid, PolarAngleAxis } from 'recharts';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { EmptyState, ErrorState, LoadingState, WarningState } from '@/components/ui-state';
import { useI18n } from '@/components/providers/i18n-provider';

type DashboardData = {
  artists: Awaited<ReturnType<typeof api.topArtists>> | null;
  tracks: Awaited<ReturnType<typeof api.topTracks>> | null;
  recent: Awaited<ReturnType<typeof api.recentlyPlayed>> | null;
  genres: Awaited<ReturnType<typeof api.statsGenres>> | null;
  timeline: Awaited<ReturnType<typeof api.wrappedTimeline>> | null;
  listening: Awaited<ReturnType<typeof api.statsListeningTime>> | null;
  audio: Awaited<ReturnType<typeof api.audioFeatures>> | null;
  wrapped: Awaited<ReturnType<typeof api.wrappedSummary>> | null;
  recs: Awaited<ReturnType<typeof api.recommendations>> | null;
};

const colors = ['#8b5cf6', '#6366f1', '#a78bfa', '#c4b5fd', '#7c3aed', '#4f46e5'];

export default function DashboardPage() {
  const session = useSession();
  const { t } = useI18n();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [data, setData] = useState<DashboardData>({
    artists: null,
    tracks: null,
    recent: null,
    genres: null,
    timeline: null,
    listening: null,
    audio: null,
    wrapped: null,
    recs: null,
  });

  useEffect(() => {
    if (!session?.userId) return;
    let mounted = true;

    (async () => {
      setLoading(true);
      setError(null);
      setWarnings([]);
      try {
        const labels = [
          'Top artists',
          'Top tracks',
          'Recently played',
          'Genre stats',
          'Listening timeline',
          'Listening time',
          'Audio features',
          'Wrapped summary',
          'Recommendations',
        ];
        const results = await Promise.allSettled([
          api.topArtists(session.userId),
          api.topTracks(session.userId),
          api.recentlyPlayed(session.userId, 20),
          api.statsGenres(session.userId),
          api.wrappedTimeline(session.userId),
          api.statsListeningTime(session.userId),
          api.audioFeatures(session.userId),
          api.wrappedSummary(session.userId),
          api.recommendations(session.userId, 'comfort', 6),
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
          setError(failures[0] || 'Failed to load dashboard');
        } else {
          setWarnings(failures);
        }

        const valueOrNull = <T,>(result: PromiseSettledResult<T>) =>
          result.status === 'fulfilled' ? result.value : null;

        setData({
          artists: valueOrNull(results[0]),
          tracks: valueOrNull(results[1]),
          recent: valueOrNull(results[2]),
          genres: valueOrNull(results[3]),
          timeline: valueOrNull(results[4]),
          listening: valueOrNull(results[5]),
          audio: valueOrNull(results[6]),
          wrapped: valueOrNull(results[7]),
          recs: valueOrNull(results[8]),
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

  const avgAudio = useMemo(() => {
    const items = data.audio?.audio_features || [];
    if (!items.length) return null;
    const totals = items.reduce(
      (acc, f) => ({
        danceability: acc.danceability + (f.danceability || 0),
        energy: acc.energy + (f.energy || 0),
        valence: acc.valence + (f.valence || 0),
      }),
      { danceability: 0, energy: 0, valence: 0 },
    );

    return [
      { metric: 'Danceability', value: totals.danceability / items.length },
      { metric: 'Energy', value: totals.energy / items.length },
      { metric: 'Valence', value: totals.valence / items.length },
    ];
  }, [data.audio]);

  if (!session?.userId) return <LoadingState text={t('loading')} />;
  if (loading) return <LoadingState text={t('loading')} />;
  if (error) return <ErrorState message={error} />;

  return (
    <div className="space-y-8">
      <h1 className="text-4xl font-bold bg-gradient-to-r from-violet-200 to-indigo-200 bg-clip-text text-transparent">{t('dashboard')}</h1>

      <div className="grid md:grid-cols-4 gap-4">
        <Metric title={t('topArtists')} value={String(data.artists?.items?.length || 0)} />
        <Metric title={t('topTracks')} value={String(data.tracks?.items?.length || 0)} />
        <Metric title={t('recentlyPlayed')} value={String(data.listening?.plays || 0)} />
        <Metric title={t('listeningTime')} value={`${Math.round(data.listening?.hours || 0)}h`} />
      </div>

      {(warnings.length > 0 || data.audio?.warning || data.recs?.warning || data.wrapped?.warning) && (
        <div className="space-y-2">
          {warnings.map((warning, idx) => (
            <WarningState key={`${warning}-${idx}`} text={warning} />
          ))}
          {data.audio?.warning && <WarningState text={`${t('warning')}: ${data.audio.warning}`} />}
          {data.recs?.warning && <WarningState text={`${t('warning')}: ${data.recs.warning}`} />}
          {data.wrapped?.warning && <WarningState text={`${t('warning')}: ${data.wrapped.warning}`} />}
        </div>
      )}

      <div className="grid lg:grid-cols-2 gap-6">
        <Panel title={t('genreDistribution')}>
          {!data.genres?.genres?.length ? (
            <EmptyState text={t('empty')} />
          ) : (
            <div className="h-72">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie data={data.genres.genres.slice(0, 8)} dataKey="count" nameKey="genre" outerRadius={100}>
                    {data.genres.genres.slice(0, 8).map((_, i) => (
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
          {!data.timeline?.by_day?.length ? (
            <EmptyState text={t('empty')} />
          ) : (
            <div className="h-72">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={data.timeline.by_day}>
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
            <EmptyState text={t('fallback')} />
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
          {!data.wrapped ? (
            <EmptyState text={t('empty')} />
          ) : (
            <div className="space-y-3 text-sm text-muted-foreground">
              <p>Top artists: {(data.wrapped.top_artists || []).slice(0, 3).map((a) => a.name).join(', ') || '-'}</p>
              <p>Top tracks: {(data.wrapped.top_tracks || []).slice(0, 3).map((t) => t.name).join(', ') || '-'}</p>
              <p>Top genres (Last.fm): {(data.wrapped.top_genres || []).slice(0, 3).map((g) => g.genre).join(', ') || '-'}</p>
              <p>Recent plays: {data.wrapped.recent_plays_count ?? '-'}</p>
            </div>
          )}
        </Panel>

        <Panel title="Recommendations">
          {!data.recs?.items?.length ? (
            <EmptyState text={data.recs?.source === 'top_tracks_fallback' ? 'Fallback: top tracks' : t('empty')} />
          ) : (
            <ul className="space-y-2 text-sm">
              {data.recs.items.slice(0, 5).map((item, idx) => (
                <li key={`${item.track?.id || idx}`}>
                  <span className="font-medium">{item.track?.name}</span>
                  <span className="text-muted-foreground text-xs block">{item.reason}</span>
                </li>
              ))}
              <li className="text-xs text-muted-foreground pt-1">Source: {data.recs.source}</li>
            </ul>
          )}
        </Panel>
      </div>

      <div className="grid lg:grid-cols-3 gap-6">
        <Panel title={t('topArtists')}>
          <ul className="space-y-2 text-sm">
            {(data.artists?.items || []).slice(0, 8).map((a, index) => (
              <li key={a.id} className="flex items-center justify-between gap-3">
                <span className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-glass-border bg-secondary/30 text-xs font-semibold text-foreground">
                  {a.position ?? index + 1}
                </span>
                <span className="flex-1">{a.name}</span>
                <span className="text-muted-foreground text-xs">
                  {a.score != null ? `score ${a.score.toFixed(1)}` : a.popularity ?? '-'}
                </span>
              </li>
            ))}
          </ul>
        </Panel>

        <Panel title={t('topTracks')}>
          <ul className="space-y-2 text-sm">
            {(data.tracks?.items || []).slice(0, 8).map((t, index) => (
              <li key={t.id} className="space-y-1">
                <div className="flex items-center gap-3">
                  <span className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-glass-border bg-secondary/30 text-xs font-semibold text-foreground">
                    {index + 1}
                  </span>
                  <span className="flex-1">{t.name}</span>
                </div>
                <div className="text-muted-foreground text-xs">{(t.artists || []).map((a) => a.name).join(', ')}</div>
              </li>
            ))}
          </ul>
        </Panel>

        <Panel title={t('recentlyPlayed')}>
          <ul className="space-y-2 text-sm">
            {(data.recent?.items || []).slice(0, 8).map((r, idx) => (
              <li key={`${r.track?.id || idx}-${idx}`} className="space-y-1">
                <div className="flex items-center gap-3">
                  <span className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-glass-border bg-secondary/30 text-xs font-semibold text-foreground">
                    {idx + 1}
                  </span>
                  <span className="flex-1">{r.track?.name || '-'}</span>
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
