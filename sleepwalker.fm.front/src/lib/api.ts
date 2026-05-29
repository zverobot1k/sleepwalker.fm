import { apiRequestHeaders, describeFetchError, getApiBase } from '@/lib/api-base';

export type ApiWarning = { warning?: string; notice?: string; source?: string };

export type ExternalURLs = Record<string, string>;

export type Image = {
  height: number;
  width: number;
  url: string;
};

export type Artist = {
  id: string;
  name: string;
  popularity?: number;
  genres?: string[];
  images?: Image[];
  external_urls?: ExternalURLs;
  position?: number;
  score?: number;
};

export type SimpleArtist = {
  id: string;
  name: string;
};

export type SimpleAlbum = {
  id?: string;
  name?: string;
  images?: Image[];
  release_date?: string;
};

export type Track = {
  id: string;
  name: string;
  uri?: string;
  popularity?: number;
  duration_ms?: number;
  artists?: SimpleArtist[];
  album?: SimpleAlbum;
  external_urls?: ExternalURLs;
};

export type PlayHistoryItem = {
  track: Track;
  played_at: string;
};

export type TopArtistsResponse = {
  items: Artist[];
  total?: number;
  limit?: number;
  source?: string;
};

export type TopTracksResponse = {
  items: Track[];
  total?: number;
  limit?: number;
};

export type RecentlyPlayedResponse = {
  items: PlayHistoryItem[];
  cursors?: { after?: string; before?: string };
  next?: string;
  limit?: number;
};

export type AudioFeature = {
  id?: string;
  danceability: number;
  energy: number;
  valence: number;
  tempo: number;
  acousticness?: number;
  instrumentalness?: number;
  liveness?: number;
  speechiness?: number;
  loudness?: number;
  duration_ms?: number;
};

export type AudioFeaturesResponse = {
  audio_features: AudioFeature[];
  warning?: string;
  notice?: string;
  source?: string;
};

export type WrappedSummaryResponse = {
  time_range?: string;
  top_artists: Artist[];
  top_tracks: Track[];
  top_genres: Array<{ genre: string; count: number; weight?: number }>;
  recent_plays_count?: number;
  recent_minutes_total?: number;
  unique_tracks_recent?: number;
  unique_artists_recent?: number;
  notice?: string;
  warning?: string;
  audio_features_warning?: string;
};

export type WrappedInsightsResponse = {
  time_range?: string;
  highlights: string[];
  top_genre?: string;
  top_artist?: string;
  top_track?: string;
  listener_tag?: string;
  notice?: string;
  warning?: string;
};

export type WrappedTimelineResponse = {
  by_day: Array<{ key: string; plays: number }>;
  by_hour: Array<{ key: string; plays: number }>;
  warning?: string;
};

export type WrappedCompareResponse = {
  left_range?: string;
  right_range?: string;
  track_overlap: number;
  artist_overlap: number;
  new_tracks_in_left?: number;
  new_artists_in_left?: number;
  left_top_track?: string;
  right_top_track?: string;
  left_top_artist?: string;
  right_top_artist?: string;
  notice?: string;
  warning?: string;
};

export type StatsProfileResponse = {
  time_range?: string;
  avg_track_popularity: number;
  avg_track_duration_ms: number;
  avg_danceability?: number;
  avg_energy?: number;
  avg_valence?: number;
  warning?: string;
};

export type StatsGenresResponse = {
  time_range?: string;
  genres: Array<{ genre: string; count: number; weight?: number }>;
  source?: string;
  notice?: string;
  warning?: string;
};

export type StatsListeningTimeResponse = {
  plays: number;
  duration_ms: number;
  minutes: number;
  hours: number;
  source_window?: string;
  warning?: string;
};

export type RecommendationsResponse = {
  mode: string;
  source: string;
  items: Array<{ track: Track; reason: string }>;
  warning?: string;
  notice?: string;
  seed_track_ids?: string[];
  seed_artist_ids?: string[];
};

export type ExportTrack = {
  id: string;
  name: string;
  artists: string[];
  uri: string;
  spotify_url?: string;
};

export type SessionStateResponse = {
  user_id: string;
  expires_at: string;
  scope?: string;
  connected: boolean;
};

export type CreatePlaylistResponse = {
  playlist_id?: string;
  playlist_url?: string;
  tracks_added: number;
  warning?: string;
  notice?: string;
  fallback_uris?: string[];
  fallback_tracks?: ExportTrack[];
};

function buildQuery(params: Record<string, string | number | boolean | undefined | null>) {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue;
    search.set(key, String(value));
  }
  const query = search.toString();
  return query ? `?${query}` : '';
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method || 'GET').toUpperCase();
  const headers = apiRequestHeaders(init?.headers);
  if (method !== 'GET' && method !== 'HEAD' && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  const url = `${getApiBase()}${path}`;

  let res: Response;
  try {
    res = await fetch(url, {
      ...init,
      headers,
      cache: 'no-store',
      mode: 'cors',
    });
  } catch (err) {
    const message = describeFetchError(url, err);
    console.error('[sleepwalker.fm] API request failed:', url, err);
    throw new Error(message);
  }

  const text = await res.text();
  let body: unknown = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = null;
    }
  }

  if (!res.ok) {
    const message =
      (body as { error?: string } | null)?.error ||
      `Request failed: ${res.status} ${res.statusText} (${url})`;
    console.error('[sleepwalker.fm] API error response:', res.status, url, body ?? text.slice(0, 200));
    throw new Error(message);
  }

  return body as T;
}

export const api = {
  session: (userId: string) =>
    request<SessionStateResponse>(`/auth/session/${encodeURIComponent(userId)}`),

  refreshTokens: (userId: string) =>
    request<{ user_id: string; expires_at: string }>(
      `/auth/refresh/${encodeURIComponent(userId)}`,
      { method: 'POST' },
    ),

  topArtists: (userId: string, timeRange = 'medium_term', limit = 12) =>
    request<TopArtistsResponse>(
      `/api/spotify/top/artists/${encodeURIComponent(userId)}${buildQuery({ time_range: timeRange, limit })}`,
    ),

  topTracks: (userId: string, timeRange = 'medium_term', limit = 12) =>
    request<TopTracksResponse>(
      `/api/spotify/top/tracks/${encodeURIComponent(userId)}${buildQuery({ time_range: timeRange, limit })}`,
    ),

  recentlyPlayed: (userId: string, limit = 20) =>
    request<RecentlyPlayedResponse>(
      `/api/spotify/recently-played/${encodeURIComponent(userId)}${buildQuery({ limit })}`,
    ),

  audioFeatures: (userId: string, timeRange = 'medium_term', limit = 20) =>
    request<AudioFeaturesResponse>(
      `/api/spotify/audio-features/${encodeURIComponent(userId)}${buildQuery({ time_range: timeRange, limit })}`,
    ),

  wrappedSummary: (userId: string, timeRange = 'medium_term', limit = 10) =>
    request<WrappedSummaryResponse>(
      `/api/wrapped/summary/${encodeURIComponent(userId)}${buildQuery({ time_range: timeRange, limit })}`,
    ),

  wrappedInsights: (userId: string, timeRange = 'medium_term') =>
    request<WrappedInsightsResponse>(
      `/api/wrapped/insights/${encodeURIComponent(userId)}${buildQuery({ time_range: timeRange })}`,
    ),

  wrappedTimeline: (userId: string) =>
    request<WrappedTimelineResponse>(
      `/api/wrapped/timeline/${encodeURIComponent(userId)}`,
    ),

  wrappedCompare: (userId: string, left = 'short_term', right = 'long_term') =>
    request<WrappedCompareResponse>(
      `/api/wrapped/compare/${encodeURIComponent(userId)}${buildQuery({ left, right })}`,
    ),

  statsProfile: (userId: string, timeRange = 'medium_term') =>
    request<StatsProfileResponse>(
      `/api/stats/profile/${encodeURIComponent(userId)}${buildQuery({ time_range: timeRange })}`,
    ),

  statsGenres: (userId: string, timeRange = 'medium_term') =>
    request<StatsGenresResponse>(
      `/api/stats/genres/${encodeURIComponent(userId)}${buildQuery({ time_range: timeRange })}`,
    ),

  statsListeningTime: (userId: string, params?: { from?: string; to?: string }) =>
    request<StatsListeningTimeResponse>(
      `/api/stats/listening-time/${encodeURIComponent(userId)}${buildQuery({ from: params?.from, to: params?.to })}`,
    ),

  recommendations: (userId: string, mode = 'comfort', limit = 12) =>
    request<RecommendationsResponse>(
      `/api/recommendations/${encodeURIComponent(userId)}${buildQuery({ mode, limit })}`,
    ),

  createPlaylist: (userId: string, limit = 20) =>
    request<CreatePlaylistResponse>(
      `/api/recommendations/playlist/${encodeURIComponent(userId)}${buildQuery({ limit })}`,
      { method: 'POST' },
    ),
};
