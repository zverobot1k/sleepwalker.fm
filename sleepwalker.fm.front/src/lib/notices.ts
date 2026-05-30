import { Language, messages } from '@/lib/messages';

const NOTICE_KEYS: Record<string, keyof (typeof messages)['en']> = {
  audio_features_estimated: 'noticeAudioFeaturesEstimated',
  recommendations_from_listening_history: 'noticeRecommendationsListening',
  recommendations_from_top_tracks: 'noticeRecommendationsTopTracks',
  recommendations_unavailable: 'noticeRecommendationsUnavailable',
  playlist_export_fallback: 'noticePlaylistExportFallback',
  playlist_scopes_restricted: 'noticePlaylistScopesRestricted',
  lastfm_unavailable: 'noticeLastFmUnavailable',
  lastfm_no_tags: 'noticeLastFmNoTags',
};

const SOURCE_KEYS: Record<string, keyof (typeof messages)['en']> = {
  spotify_recommendations: 'sourceSpotifyRecommendations',
  lastfm_similar_artists: 'sourceLastFm',
  top_tracks_fallback: 'sourceTopTracks',
  derived: 'sourceDerived',
  lastfm: 'sourceLastFmGenres',
  recommendations_unavailable: 'noticeRecommendationsUnavailable',
};

/** Maps API notice codes and legacy warning strings to localized copy. */
export function resolveNotice(
  lang: Language,
  notice?: string | null,
  warning?: string | null,
): string | null {
  const code = notice?.trim();
  if (code) {
    const key = NOTICE_KEYS[code];
    if (key) return messages[lang][key];
  }

  const legacy = warning?.trim();
  if (!legacy) return null;

  const lower = legacy.toLowerCase();
  if (lower.includes('audio-features') || lower.includes('audio features')) {
    return messages[lang].noticeAudioFeaturesEstimated;
  }
  if (lower.includes('recommendations 404') || lower.includes('recommendations unavailable')) {
    return messages[lang].noticeRecommendationsListening;
  }
  if (lower.includes('top tracks') || lower.includes('fallback')) {
    return messages[lang].noticeRecommendationsTopTracks;
  }
  if (lower.includes('playlist') && (lower.includes('forbidden') || lower.includes('scope'))) {
    return messages[lang].noticePlaylistScopesRestricted;
  }
  if (lower.includes('last.fm') && lower.includes('unavailable')) {
    return messages[lang].noticeLastFmUnavailable;
  }

  return null;
}

export function resolveSourceLabel(lang: Language, source?: string | null): string {
  if (!source) return '—';
  const key = SOURCE_KEYS[source];
  if (key) return messages[lang][key];
  return source;
}

export function resolveReason(lang: Language, reason?: string | null): string {
  if (!reason) return '';
  if (reason === 'top_tracks_fallback') {
    return messages[lang].reasonTopTracks;
  }
  const prefix = 'based on Last.fm similar artist:';
  if (reason.startsWith(prefix)) {
    const artist = reason.slice(prefix.length).trim();
    return messages[lang].reasonLastFmSimilar.replace('{artist}', artist);
  }
  if (reason.startsWith('fallback:')) {
    return messages[lang].reasonTopTracks;
  }
  return reason;
}

const RU_GENRE_TRANSLATIONS: Record<string, string> = {
  rap: 'Рэп',
  'hip hop': 'Хип-хоп',
  rock: 'Рок',
  pop: 'Поп',
  jazz: 'Джаз',
  electronic: 'Электроника',
  metal: 'Метал',
  punk: 'Панк',
  indie: 'Инди',
  blues: 'Блюз',
  soul: 'Соул',
  folk: 'Фолк',
  classical: 'Классика',
  ambient: 'Эмбиент',
  techno: 'Техно',
  house: 'Хаус',
  trance: 'Транс',
  rnb: 'РнБ',
  trap: 'Трэп',
};

function titleCaseWords(value: string): string {
  return value
    .split(' ')
    .map((part) => (part ? part[0].toUpperCase() + part.slice(1) : part))
    .join(' ');
}

function localizeGenreLabel(lang: Language, genre: string): string {
  const normalized = genre.trim().toLowerCase().replace(/[_-]+/g, ' ').replace(/\s+/g, ' ');
  if (!normalized) return '';
  if (lang === 'ru') {
    const translated = RU_GENRE_TRANSLATIONS[normalized];
    if (translated) return translated;
  }
  return titleCaseWords(normalized);
}

export function formatGenreDisplay(lang: Language, rawGenre?: string | null): string {
  if (!rawGenre) return '';
  const value = rawGenre.trim();
  if (!value) return '';
  if (value === 'null' || value === 'undefined') return '';

  const mergedPattern = /^(.*?)\s*(?:via\s+)?based on Last\.fm similar artist:\s*(.+)$/i;
  const match = value.match(mergedPattern);

  if (match) {
    const primary = localizeGenreLabel(lang, match[1] || '') || localizeGenreLabel(lang, value);
    const artist = (match[2] || '').trim();
    if (!artist) return primary;
    return `${primary} (Last.fm: ${artist})`;
  }

  return localizeGenreLabel(lang, value);
}
