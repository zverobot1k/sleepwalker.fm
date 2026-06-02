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
  lastfm_genre_engine: 'sourceLastFmGenreEngine',
  lastfm_taste_graph: 'sourceLastFmGenreEngine',
  top_tracks_fallback: 'sourceTopTracks',
  spotify_fallback: 'sourceSpotifyFallback',
  spotify_artist_genres: 'sourceSpotifyFallback',
  default_genres: 'sourceSpotifyFallback',
  derived: 'sourceDerived',
  lastfm: 'sourceLastFmGenres',
  spotify_snapshot: 'sourceSpotifySnapshot',
  computed: 'sourceSpotifySnapshot',
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

  // Exact matches
  if (reason === 'top_tracks_fallback' || reason === 'from your top tracks') {
    return messages[lang].reasonTopTracks;
  }
  if (reason === 'from your recently played') {
    return messages[lang].reasonTopTracks;
  }
  if (
    reason === 'explore expansion' ||
    reason === 'graph exploration' ||
    reason === 'graph backfill' ||
    reason === 'adjacent cluster'
  ) {
    return messages[lang].reasonExplore;
  }

  // "based on Last.fm artist: X" (from buildRecommendationsFromSimilarArtists)
  const lastfmArtistPrefix = 'based on Last.fm artist:';
  if (reason.startsWith(lastfmArtistPrefix)) {
    const artist = reason.slice(lastfmArtistPrefix.length).trim();
    if (artist) return messages[lang].reasonLastFmSimilar.replace('{artist}', artist);
  }

  // "based on genre X" (from buildItems)
  const genrePrefix = 'based on genre ';
  if (reason.startsWith(genrePrefix)) {
    const genre = reason.slice(genrePrefix.length).trim();
    const localized = formatGenreDisplay(lang, genre);
    return messages[lang].reasonBasedOnGenre.replace('{genre}', localized || genre);
  }

  // "genre X artist Y" (from buildItems)
  const genreArtistMatch = reason.match(/^genre\s+(.+?)\s+artist\s+(.+)$/i);
  if (genreArtistMatch) {
    const artist = genreArtistMatch[2].trim();
    return messages[lang].reasonLastFmSimilar.replace('{artist}', artist);
  }

  // "top artist X" (from buildItems)
  const topArtistPrefix = 'top artist ';
  if (reason.startsWith(topArtistPrefix)) {
    const artist = reason.slice(topArtistPrefix.length).trim();
    if (artist) return messages[lang].reasonTopArtist.replace('{artist}', artist);
  }

  // "similar to X" (from buildItems)
  const similarPrefix = 'similar to ';
  if (reason.startsWith(similarPrefix)) {
    const artist = reason.slice(similarPrefix.length).trim();
    if (artist) return messages[lang].reasonSimilarTo.replace('{artist}', artist);
  }

  // "same cluster X" (from recommendViaLastFM)
  const sameClusterMatch = reason.match(/^same cluster\s+(.+)$/i);
  if (sameClusterMatch) {
    const genre = sameClusterMatch[1].trim();
    const localized = formatGenreDisplay(lang, genre);
    return messages[lang].reasonBasedOnGenre.replace('{genre}', localized || genre);
  }

  // Fallback: return as-is
  return reason;
}

// ─── Genre translation ────────────────────────────────────────────────────────

const RU_GENRE_MAP: Record<string, string> = {
  // Core genres
  'rock': 'Рок',
  'pop': 'Поп',
  'jazz': 'Джаз',
  'blues': 'Блюз',
  'soul': 'Соул',
  'folk': 'Фолк',
  'classical': 'Классика',
  'opera': 'Опера',
  'country': 'Кантри',
  'reggae': 'Регги',
  'ska': 'Ска',
  'funk': 'Фанк',
  'disco': 'Диско',

  // Hip-hop / rap
  'hip hop': 'Хип-хоп',
  'rap': 'Рэп',
  'trap': 'Трэп',
  'drill': 'Дрилл',
  'grime': 'Грайм',

  // Electronic
  'electronic': 'Электроника',
  'electronica': 'Электроника',
  'techno': 'Техно',
  'house': 'Хаус',
  'deep house': 'Дип-хаус',
  'tech house': 'Тех-хаус',
  'trance': 'Транс',
  'dubstep': 'Дабстеп',
  'drum and bass': 'Драм-н-бейс',
  'dnb': 'Драм-н-бейс',
  'ambient': 'Эмбиент',
  'idm': 'IDM',
  'edm': 'EDM',
  'synthwave': 'Синтвейв',
  'synthpop': 'Синт-поп',
  'electropop': 'Электро-поп',
  'chillout': 'Чиллаут',
  'chillwave': 'Чиллвейв',
  'lo fi': 'Лоу-фай',
  'lofi': 'Лоу-фай',
  'lo-fi': 'Лоу-фай',
  'vaporwave': 'Вейпорвейв',

  // Rock subgenres
  'alternative': 'Альтернатива',
  'alternative rock': 'Альтернативный рок',
  'indie': 'Инди',
  'indie rock': 'Инди-рок',
  'indie pop': 'Инди-поп',
  'post rock': 'Пост-рок',
  'post-rock': 'Пост-рок',
  'punk': 'Панк',
  'punk rock': 'Панк-рок',
  'post punk': 'Пост-панк',
  'post-punk': 'Пост-панк',
  'new wave': 'Нью-вейв',
  'metal': 'Метал',
  'heavy metal': 'Хэви-метал',
  'death metal': 'Дэт-метал',
  'black metal': 'Блэк-метал',
  'thrash metal': 'Трэш-метал',
  'doom metal': 'Дум-метал',
  'prog rock': 'Прог-рок',
  'progressive rock': 'Прогрессив-рок',
  'psychedelic rock': 'Психоделический рок',
  'psychedelic': 'Психоделика',
  'grunge': 'Гранж',
  'shoegaze': 'Шугейзинг',
  'dream pop': 'Дрим-поп',
  'emo': 'Эмо',
  'hardcore': 'Хардкор',
  'noise rock': 'Нойз-рок',
  'art rock': 'Арт-рок',
  'glam rock': 'Глэм-рок',
  'gothic rock': 'Готик-рок',
  'stoner rock': 'Стоунер-рок',

  // R&B
  'rnb': 'РнБ',
  'r&b': 'РнБ',
  'rhythm and blues': 'Ритм-н-блюз',
  'neo soul': 'Нео-соул',

  // World / regional
  'latin': 'Латин',
  'bossa nova': 'Босса-нова',
  'afrobeat': 'Афробит',
  'k-pop': 'К-поп',
  'j-pop': 'Дж-поп',
  'j-rock': 'Дж-рок',
  'anime': 'Аниме',

  // Misc
  'singer-songwriter': 'Авторская песня',
  'acoustic': 'Акустика',
  'instrumental': 'Инструментал',
  'experimental': 'Экспериментальная',
  'noise': 'Нойз',
  'spoken word': 'Разговорный жанр',
  'a cappella': 'А капелла',
  'gospel': 'Госпел',
  'new age': 'Нью-эйдж',
};

// Clean display names for English (title-case normalisation)
const EN_GENRE_DISPLAY: Record<string, string> = {
  'hip hop': 'Hip-Hop',
  'rnb': 'R&B',
  'drum and bass': 'Drum & Bass',
  'dnb': 'Drum & Bass',
  'lo fi': 'Lo-Fi',
  'lofi': 'Lo-Fi',
  'idm': 'IDM',
  'edm': 'EDM',
  'k-pop': 'K-Pop',
  'j-pop': 'J-Pop',
  'j-rock': 'J-Rock',
};

function titleCaseWords(value: string): string {
  return value
    .split(' ')
    .map((part) => (part ? part[0].toUpperCase() + part.slice(1) : part))
    .join(' ');
}

function localizeGenreLabel(lang: Language, genre: string): string {
  const normalized = genre.trim().toLowerCase().replace(/[_]+/g, ' ').replace(/\s+/g, ' ');
  if (!normalized) return '';

  if (lang === 'ru') {
    const translated = RU_GENRE_MAP[normalized];
    if (translated) return translated;
    // Attempt word-by-word for compound genres not in map
    return titleCaseWords(normalized);
  }

  // English: use curated display name or fall back to title-case
  const display = EN_GENRE_DISPLAY[normalized];
  if (display) return display;
  return titleCaseWords(normalized);
}

export function formatGenreDisplay(lang: Language, rawGenre?: string | null): string {
  if (!rawGenre) return '';
  const value = rawGenre.trim();
  if (!value || value === 'null' || value === 'undefined' || value === 'unknown') return '';
  return localizeGenreLabel(lang, value);
}
