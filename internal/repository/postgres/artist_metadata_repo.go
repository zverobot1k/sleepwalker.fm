package postgres

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm/clause"
)

type ArtistMetadata struct {
	ArtistID           string    `gorm:"primaryKey;column:artist_id"`
	GenresJSON         string    `gorm:"column:genres_json;type:text;not null"`
	InferredGenresJSON string    `gorm:"column:inferred_genres_json;type:text;not null"`
	RelatedArtistsJSON string    `gorm:"column:related_artists_json;type:text;not null"`
	Source             string    `gorm:"column:source;not null"`
	UpdatedAt          time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ArtistMetadata) TableName() string {
	return "artist_metadata"
}

type ArtistMetadataRecord struct {
	ArtistID       string
	Genres         []string
	InferredGenres []string
	RelatedArtists []string
	Source         string
	UpdatedAt      time.Time
}

type ArtistMetadataRepo struct {
	DB *DB
}

func NewArtistMetadataRepo(db *DB) *ArtistMetadataRepo {
	return &ArtistMetadataRepo{DB: db}
}

func (r *ArtistMetadataRepo) Upsert(ctx context.Context, record ArtistMetadataRecord) error {
	genresJSON, err := json.Marshal(safeStrings(record.Genres))
	if err != nil {
		return err
	}
	inferredJSON, err := json.Marshal(safeStrings(record.InferredGenres))
	if err != nil {
		return err
	}
	relatedJSON, err := json.Marshal(safeStrings(record.RelatedArtists))
	if err != nil {
		return err
	}

	row := ArtistMetadata{
		ArtistID:           record.ArtistID,
		GenresJSON:         string(genresJSON),
		InferredGenresJSON: string(inferredJSON),
		RelatedArtistsJSON: string(relatedJSON),
		Source:             record.Source,
		UpdatedAt:          time.Now(),
	}

	return r.DB.Gorm.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "artist_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"genres_json", "inferred_genres_json", "related_artists_json", "source", "updated_at"}),
	}).Create(&row).Error
}

func (r *ArtistMetadataRepo) GetByArtistID(ctx context.Context, artistID string) (ArtistMetadataRecord, bool, error) {
	var row ArtistMetadata
	err := r.DB.Gorm.WithContext(ctx).Where("artist_id = ?", artistID).First(&row).Error
	if err != nil {
		return ArtistMetadataRecord{}, false, err
	}

	record, err := row.toRecord()
	if err != nil {
		return ArtistMetadataRecord{}, false, err
	}
	return record, true, nil
}

func (m ArtistMetadata) toRecord() (ArtistMetadataRecord, error) {
	genres := []string{}
	inferred := []string{}
	related := []string{}

	if err := json.Unmarshal([]byte(m.GenresJSON), &genres); err != nil {
		return ArtistMetadataRecord{}, err
	}
	if err := json.Unmarshal([]byte(m.InferredGenresJSON), &inferred); err != nil {
		return ArtistMetadataRecord{}, err
	}
	if err := json.Unmarshal([]byte(m.RelatedArtistsJSON), &related); err != nil {
		return ArtistMetadataRecord{}, err
	}

	return ArtistMetadataRecord{
		ArtistID:       m.ArtistID,
		Genres:         safeStrings(genres),
		InferredGenres: safeStrings(inferred),
		RelatedArtists: safeStrings(related),
		Source:         m.Source,
		UpdatedAt:      m.UpdatedAt,
	}, nil
}

func safeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
