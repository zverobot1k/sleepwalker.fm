'use client';

import { useState } from 'react';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { ErrorState, LoadingState } from '@/components/ui-state';
import { useI18n } from '@/components/providers/i18n-provider';
import { PlaylistExportResult } from '@/components/playlist-export';

export default function PlaylistPage() {
  const session = useSession();
  const { t } = useI18n();
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Awaited<ReturnType<typeof api.createPlaylist>> | null>(null);

  if (!session?.userId) return <LoadingState text={t('loading')} />;

  const create = async () => {
    setCreating(true);
    setError(null);
    try {
      const response = await api.createPlaylist(session.userId, 20);
      setResult(response);
    } catch (e) {
      setError(e instanceof Error ? e.message : t('failedPlaylist'));
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="space-y-6 max-w-3xl">
      <h1 className="text-4xl font-bold bg-gradient-to-r from-violet-200 to-indigo-200 bg-clip-text text-transparent">
        {t('playlist')}
      </h1>

      <button
        onClick={create}
        disabled={creating}
        className="px-6 py-3 rounded-lg bg-gradient-to-r from-violet-600 to-indigo-600 hover:from-violet-500 hover:to-indigo-500 disabled:opacity-50"
      >
        {creating ? t('creating') : t('createPlaylist')}
      </button>

      {error && <ErrorState message={error} />}

      {result?.playlist_url && (
        <a
          href={result.playlist_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center justify-center rounded-xl border border-emerald-500/35 bg-[#121212] px-6 py-3 text-sm font-semibold text-emerald-300 shadow-[0_10px_24px_rgba(16,185,129,0.18)] transition hover:-translate-y-0.5 hover:bg-[#1a1a1a] hover:text-emerald-200 active:translate-y-0 active:bg-[#0f0f0f]"
        >
          {t('openPlaylist')}
        </a>
      )}

      {result && <PlaylistExportResult result={result} />}
    </div>
  );
}
