'use client';

import { useState } from 'react';
import { api } from '@/lib/api';
import { useSession } from '@/hooks/use-session';
import { ErrorState, LoadingState, WarningState } from '@/components/ui-state';

export default function PlaylistPage() {
  const session = useSession();
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Awaited<ReturnType<typeof api.createPlaylist>> | null>(null);

  if (!session?.userId) return <LoadingState />;

  const create = async () => {
    setCreating(true);
    setError(null);
    try {
      const response = await api.createPlaylist(session.userId, 20);
      setResult(response);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create playlist');
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="space-y-6 max-w-3xl">
      <h1 className="text-4xl font-bold bg-gradient-to-r from-violet-200 to-indigo-200 bg-clip-text text-transparent">Playlist Export</h1>

      <button
        onClick={create}
        disabled={creating}
        className="px-6 py-3 rounded-lg bg-gradient-to-r from-violet-600 to-indigo-600 hover:from-violet-500 hover:to-indigo-500 disabled:opacity-50"
      >
        {creating ? 'Creating...' : 'Create playlist from recommendations'}
      </button>

      {error && <ErrorState message={error} />}
      {result?.warning && <WarningState text={result.warning} />}

      {result?.playlist_url && (
        <a href={result.playlist_url} target="_blank" rel="noreferrer" className="inline-block text-violet-300 underline">
          Open created playlist in Spotify
        </a>
      )}

      {!!result?.fallback_uris?.length && (
        <div className="p-4 rounded-xl border border-glass-border bg-glass-bg">
          <p className="text-sm text-muted-foreground mb-2">Spotify playlist creation unavailable. Use fallback URIs:</p>
          <pre className="text-xs overflow-x-auto whitespace-pre-wrap">{result.fallback_uris.join('\n')}</pre>
        </div>
      )}
    </div>
  );
}
