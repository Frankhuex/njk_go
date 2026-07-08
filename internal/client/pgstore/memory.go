package pgstore

import (
	"context"
	"time"

	"njk_go/internal/dal/model"

	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

const (
	memoryFactTable       = "memory_fact"
	memoryImpressionTable = "memory_impression"
)

type MemorySearchHit struct {
	ID         int64     `gorm:"column:id"`
	UserID     *string   `gorm:"column:user_id"`
	MessageID  *string   `gorm:"column:message_id"`
	Content    string    `gorm:"column:content"`
	Confidence float32   `gorm:"column:confidence"`
	Similarity float64   `gorm:"column:similarity"`
	Version    int32     `gorm:"column:version"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (s *Store) SaveMemoryFact(ctx context.Context, record *model.MemoryFact) error {
	if record == nil {
		return nil
	}
	return s.db.WithContext(ctx).Create(record).Error
}

func (s *Store) SaveMemoryImpression(ctx context.Context, record *model.MemoryImpression) error {
	if record == nil {
		return nil
	}
	return s.db.WithContext(ctx).Create(record).Error
}

func (s *Store) SearchFactMemories(ctx context.Context, groupID string, userID *string, vector pgvector.Vector, limit int, minSimilarity float64) ([]MemorySearchHit, error) {
	if groupID == "" || limit <= 0 {
		return nil, nil
	}
	query := `
SELECT
    id,
    user_id,
    message_id,
    content,
    confidence,
    1 - (embedding <=> ?) AS similarity,
    0 AS version,
    created_at
FROM memory_fact
WHERE group_id = ?
  AND is_active = TRUE
  AND (1 - (embedding <=> ?)) >= ?
`
	args := []any{vector, groupID, vector, minSimilarity}
	if userID == nil {
		query += "  AND user_id IS NULL\n"
	} else {
		query += "  AND user_id = ?\n"
		args = append(args, *userID)
	}
	query += "ORDER BY embedding <=> ?, created_at DESC LIMIT ?"
	args = append(args, vector, limit)

	var rows []MemorySearchHit
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) FindNearestFactMemory(ctx context.Context, groupID string, userID *string, vector pgvector.Vector) (*MemorySearchHit, error) {
	rows, err := s.SearchFactMemories(ctx, groupID, userID, vector, 1, 0)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func (s *Store) SearchImpressionMemories(ctx context.Context, groupID string, userID string, vector pgvector.Vector, limit int, minSimilarity float64) ([]MemorySearchHit, error) {
	if groupID == "" || userID == "" || limit <= 0 {
		return nil, nil
	}
	query := `
SELECT
    id,
    user_id,
    message_id,
    content,
    confidence,
    1 - (embedding <=> ?) AS similarity,
    version,
    created_at
FROM memory_impression
WHERE group_id = ?
  AND user_id = ?
  AND is_active = TRUE
  AND (1 - (embedding <=> ?)) >= ?
ORDER BY embedding <=> ?, version DESC, created_at DESC
LIMIT ?
`
	var rows []MemorySearchHit
	if err := s.db.WithContext(ctx).Raw(query, vector, groupID, userID, vector, minSimilarity, vector, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) FindNearestImpression(ctx context.Context, groupID string, userID string, vector pgvector.Vector) (*MemorySearchHit, error) {
	rows, err := s.SearchImpressionMemories(ctx, groupID, userID, vector, 1, 0)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func (s *Store) MemoryFactExistsByHash(ctx context.Context, groupID string, userID *string, contentHash string) (bool, error) {
	if groupID == "" || contentHash == "" {
		return false, nil
	}
	db := s.db.WithContext(ctx).Table(memoryFactTable).Where("group_id = ? AND content_hash = ? AND is_active = TRUE", groupID, contentHash)
	if userID == nil {
		db = db.Where("user_id IS NULL")
	} else {
		db = db.Where("user_id = ?", *userID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) MemoryImpressionExistsByHash(ctx context.Context, groupID string, userID string, contentHash string) (bool, error) {
	if groupID == "" || userID == "" || contentHash == "" {
		return false, nil
	}
	var count int64
	err := s.db.WithContext(ctx).Table(memoryImpressionTable).
		Where("group_id = ? AND user_id = ? AND content_hash = ? AND is_active = TRUE", groupID, userID, contentHash).
		Count(&count).Error
	return count > 0, err
}

func (s *Store) LatestImpressionVersion(ctx context.Context, groupID string, userID string) (int32, error) {
	if groupID == "" || userID == "" {
		return 0, nil
	}
	type row struct {
		Version int32 `gorm:"column:version"`
	}
	var result row
	err := s.db.WithContext(ctx).Table(memoryImpressionTable).
		Select("COALESCE(MAX(version), 0) AS version").
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Scan(&result).Error
	if err != nil {
		return 0, err
	}
	return result.Version, nil
}

func (s *Store) DeactivateMemoryImpression(ctx context.Context, id int64) error {
	if id == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Table(memoryImpressionTable).
		Where("id = ?", id).
		Updates(map[string]any{
			"is_active":  false,
			"updated_at": time.Now(),
		}).Error
}

func (s *Store) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}
