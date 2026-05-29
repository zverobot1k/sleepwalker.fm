package handlers

// User-facing notice codes (localized on the frontend).
const (
	NoticeAudioFeaturesEstimated      = "audio_features_estimated"
	NoticeRecommendationsListening    = "recommendations_from_listening_history"
	NoticeRecommendationsTopTracks    = "recommendations_from_top_tracks"
	NoticeRecommendationsUnavailable  = "recommendations_unavailable"
	NoticePlaylistExportFallback      = "playlist_export_fallback"
	NoticePlaylistScopesRestricted    = "playlist_scopes_restricted"
	NoticeLastFmUnavailable           = "lastfm_unavailable"
	NoticeLastFmNoTags                = "lastfm_no_tags"
)

func mapGenreWarningToNotice(warning string) string {
	switch warning {
	case "last.fm unavailable":
		return NoticeLastFmUnavailable
	case "no last.fm tags found":
		return NoticeLastFmNoTags
	default:
		if warning != "" {
			return NoticeLastFmNoTags
		}
		return ""
	}
}
