'use client';

import { useMemo, useState } from 'react';
import { useI18n } from '@/components/providers/i18n-provider';
import { InfoBanner } from '@/components/info-banner';
import { resolveNotice } from '@/lib/notices';
import type { CreatePlaylistResponse } from '@/lib/api';

type ExportTrack = NonNullable<CreatePlaylistResponse['fallback_tracks']>[number];

function buildLines(tracks: ExportTrack[]) {
  return tracks.map((t) => {
    const artists = (t.artists || []).join(', ');
    return `${t.name} — ${artists}`;
  });
}

function downloadFile(filename: string, content: string, mime: string) {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export function PlaylistExportResult({ result }: { result: CreatePlaylistResponse }) {
  const { t, lang } = useI18n();
  const [copied, setCopied] = useState(false);

  const tracks: ExportTrack[] = useMemo(() => {
    if (result.fallback_tracks?.length) return result.fallback_tracks;
    return (result.fallback_uris || []).map((uri, idx) => ({
      id: String(idx),
      name: uri,
      artists: [],
      uri,
      spotify_url: uri.startsWith('spotify:track:')
        ? `https://open.spotify.com/track/${uri.replace('spotify:track:', '')}`
        : undefined,
    }));
  }, [result]);

  if (result.playlist_url && (result.tracks_added ?? 0) > 0) {
    return null;
  }

  const notice = resolveNotice(lang, result.notice, result.warning);

  if (!tracks.length && !notice) return null;

  const lines = buildLines(tracks);
  const textBlock = lines.join('\n');

  const copyAll = async () => {
    await navigator.clipboard.writeText(textBlock);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-4">
      {notice && <InfoBanner text={notice} />}

      {tracks.length > 0 && (
        <>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={copyAll}
              className="px-4 py-2 rounded-lg border border-glass-border bg-secondary/40 text-sm hover:bg-secondary/60"
            >
              {copied ? t('copied') : t('copyAll')}
            </button>
            <button
              type="button"
              onClick={() => downloadFile('sleepwalker-tracks.txt', textBlock, 'text/plain;charset=utf-8')}
              className="px-4 py-2 rounded-lg border border-glass-border bg-secondary/40 text-sm hover:bg-secondary/60"
            >
              {t('exportTxt')}
            </button>
            <button
              type="button"
              onClick={() => {
                const csv = ['track,artists,uri,spotify_url', ...tracks.map((tr) => {
                  const artists = (tr.artists || []).join('; ');
                  const esc = (v: string) => `"${v.replace(/"/g, '""')}"`;
                  return [esc(tr.name), esc(artists), esc(tr.uri || ''), esc(tr.spotify_url || '')].join(',');
                })].join('\n');
                downloadFile('sleepwalker-tracks.csv', csv, 'text/csv;charset=utf-8');
              }}
              className="px-4 py-2 rounded-lg border border-glass-border bg-secondary/40 text-sm hover:bg-secondary/60"
            >
              {t('exportCsv')}
            </button>
            <button
              type="button"
              onClick={() => downloadFile('sleepwalker-tracks.json', JSON.stringify(tracks, null, 2), 'application/json')}
              className="px-4 py-2 rounded-lg border border-glass-border bg-secondary/40 text-sm hover:bg-secondary/60"
            >
              {t('exportJson')}
            </button>
          </div>

          <div className="p-4 rounded-xl border border-glass-border bg-glass-bg">
            <h2 className="text-sm font-semibold mb-3">{t('tracksToExport')}</h2>
            <ul className="space-y-3 text-sm">
              {tracks.map((track) => (
                <li key={track.id + track.uri} className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <p className="font-medium">{track.name}</p>
                    {!!track.artists?.length && (
                      <p className="text-xs text-muted-foreground">{track.artists.join(', ')}</p>
                    )}
                  </div>
                  {track.spotify_url && (
                    <a
                      href={track.spotify_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-xs text-violet-300 hover:underline shrink-0"
                    >
                      {t('openInSpotify')}
                    </a>
                  )}
                </li>
              ))}
            </ul>
          </div>
        </>
      )}
    </div>
  );
}
