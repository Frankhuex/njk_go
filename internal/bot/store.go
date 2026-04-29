package bot

import (
	"context"
	"fmt"
	"time"

	"njk_go/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) UpsertUser(ctx context.Context, userID string, nickname string) error {
	if userID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"nickname"}),
	}).Create(&model.User{
		UserID:   userID,
		Nickname: nickname,
	}).Error
}

func (s *Store) UpsertGroup(ctx context.Context, groupID string, groupName string) error {
	if groupID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "group_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"group_name"}),
	}).Create(&model.Group{
		GroupID:   groupID,
		GroupName: groupName,
	}).Error
}

func (s *Store) FindUser(ctx context.Context, userID string) (*model.User, error) {
	if userID == "" {
		return nil, nil
	}
	var user model.User
	err := s.db.WithContext(ctx).First(&user, "user_id = ?", userID).Error
	if err == nil {
		return &user, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *Store) FindMessage(ctx context.Context, messageID string) (*model.Message, error) {
	if messageID == "" {
		return nil, nil
	}
	var message model.Message
	err := s.db.WithContext(ctx).First(&message, "message_id = ?", messageID).Error
	if err == nil {
		return &message, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *Store) SaveMessage(ctx context.Context, message *model.Message) error {
	return s.db.WithContext(ctx).Create(message).Error
}

func (s *Store) SaveAtUser(ctx context.Context, messageID string, userID string) error {
	if messageID == "" || userID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model.AtUser{
		MessageID: messageID,
		UserID:    userID,
	}).Error
}

func (s *Store) SaveImage(ctx context.Context, messageID string, imageHash string) (*model.Image, error) {
	record := &model.Image{
		MessageID: messageID,
		ImageHash: imageHash,
	}
	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Store) IsHashWhitelisted(ctx context.Context, imageHash string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.ImgWhitelist{}).Where("image_hash = ?", imageHash).Count(&count).Error
	return count > 0, err
}

func (s *Store) AddWhitelistHash(ctx context.Context, imageHash string) error {
	if imageHash == "" {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ImgWhitelist{
		ImageHash: imageHash,
	}).Error
}

func (s *Store) RecentMessages(ctx context.Context, groupID string, limit int) ([]StoredMessage, error) {
	var rows []StoredMessage
	err := s.db.WithContext(ctx).
		Table(`message AS m`).
		Select(`m.message_id, m.time, COALESCE(m.sender_id, '') AS sender_id, COALESCE(u.nickname, '') AS nickname, COALESCE(m.card, '') AS card, COALESCE(m.text, '') AS text, COALESCE(m.raw_message, '') AS raw_message, COALESCE(m.raw_json::text, '') AS raw_json`).
		Joins(`LEFT JOIN "user" u ON m.sender_id = u.user_id`).
		Where("m.group_id = ?", groupID).
		Order("m.time DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	reverseStoredMessages(rows)
	return rows, nil
}

func (s *Store) MessagesSince(ctx context.Context, groupID string, start time.Time) ([]StoredMessage, error) {
	var rows []StoredMessage
	err := s.db.WithContext(ctx).
		Table(`message AS m`).
		Select(`m.message_id, m.time, COALESCE(m.sender_id, '') AS sender_id, COALESCE(u.nickname, '') AS nickname, COALESCE(m.card, '') AS card, COALESCE(m.text, '') AS text, COALESCE(m.raw_message, '') AS raw_message, COALESCE(m.raw_json::text, '') AS raw_json`).
		Joins(`LEFT JOIN "user" u ON m.sender_id = u.user_id`).
		Where("m.group_id = ? AND m.time >= ?", groupID, start).
		Order("m.time ASC").
		Scan(&rows).Error
	return rows, err
}

func (s *Store) GroupImageCandidates(ctx context.Context, groupID string, excludeID int32) ([]StoredImage, error) {
	var rows []StoredImage
	err := s.db.WithContext(ctx).
		Table(`image AS i`).
		Select(`i.id, i.message_id, i.image_hash, m.time, COALESCE(m.sender_id, '') AS sender_id, COALESCE(u.nickname, '') AS nickname, COALESCE(m.card, '') AS card`).
		Joins(`JOIN message m ON i.message_id = m.message_id`).
		Joins(`LEFT JOIN "user" u ON m.sender_id = u.user_id`).
		Where("m.group_id = ? AND i.id <> ?", groupID, excludeID).
		Scan(&rows).Error
	return rows, err
}

func (s *Store) ReportStats(ctx context.Context, groupID string, start time.Time, limit int) (*ReportStats, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&model.Message{}).Where("group_id = ? AND time >= ?", groupID, start).Count(&total).Error; err != nil {
		return nil, err
	}

	var group model.Group
	if err := s.db.WithContext(ctx).First(&group, "group_id = ?", groupID).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	var days []ReportDay
	if err := s.db.WithContext(ctx).
		Table("message").
		Select(`DATE(time - interval '5 hours') AS date, COUNT(*) AS count`).
		Where("group_id = ? AND time >= ?", groupID, start).
		Group(`DATE(time - interval '5 hours')`).
		Order("count DESC").
		Limit(limit).
		Scan(&days).Error; err != nil {
		return nil, err
	}

	var rawNight []nightRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT ranked.time, ranked.card, ranked.sender_id, COALESCE(u.nickname, '') AS nickname
		FROM (
			SELECT m.time,
			       m.card,
			       m.sender_id,
			       ROW_NUMBER() OVER (PARTITION BY DATE(m.time - interval '5 hours') ORDER BY m.time DESC) AS rn
			FROM message m
			WHERE m.group_id = ? AND m.time >= ?
		) ranked
		LEFT JOIN "user" u ON ranked.sender_id = u.user_id
		WHERE ranked.rn = 1
	`, groupID, start).Scan(&rawNight).Error; err != nil {
		return nil, err
	}

	nightRank := rankNightRows(rawNight, limit)

	var atted []ReportAtUser
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(u.nickname, '') AS nickname, u.user_id, COUNT(a.id) AS count
		FROM at_user a
		JOIN message m ON a.message_id = m.message_id
		JOIN "user" u ON a.user_id = u.user_id
		WHERE m.group_id = ? AND m.time >= ?
		GROUP BY u.user_id, u.nickname
		ORDER BY count DESC
		LIMIT ?
	`, groupID, start, limit).Scan(&atted).Error; err != nil {
		return nil, err
	}

	return &ReportStats{
		GroupName:       group.GroupName,
		MessageCount:    total,
		TopChattedDates: days,
		LatestChatted:   nightRank,
		TopAttedUsers:   atted,
		StartDate:       start,
		EndDate:         time.Now(),
	}, nil
}

type StoredMessage struct {
	MessageID  string    `gorm:"column:message_id"`
	Time       time.Time `gorm:"column:time"`
	SenderID   string    `gorm:"column:sender_id"`
	Nickname   string    `gorm:"column:nickname"`
	Card       string    `gorm:"column:card"`
	Text       string    `gorm:"column:text"`
	RawMessage string    `gorm:"column:raw_message"`
	RawJSON    string    `gorm:"column:raw_json"`
}

func (m StoredMessage) Format() string {
	name := m.Card
	if name == "" {
		name = m.Nickname
	}
	if name == "" {
		name = "Unknown User"
	}
	return fmt.Sprintf("[%s] %s (%s): %s", formatDisplayTime(m.Time), name, m.SenderID, m.Text)
}

type StoredImage struct {
	ID        int32     `gorm:"column:id"`
	MessageID string    `gorm:"column:message_id"`
	ImageHash string    `gorm:"column:image_hash"`
	Time      time.Time `gorm:"column:time"`
	SenderID  string    `gorm:"column:sender_id"`
	Nickname  string    `gorm:"column:nickname"`
	Card      string    `gorm:"column:card"`
}

type ReportStats struct {
	GroupName       string
	MessageCount    int64
	TopChattedDates []ReportDay
	LatestChatted   []ReportNight
	TopAttedUsers   []ReportAtUser
	StartDate       time.Time
	EndDate         time.Time
}

type ReportDay struct {
	Date  time.Time `gorm:"column:date"`
	Count int64     `gorm:"column:count"`
}

type ReportNight struct {
	FullTime string
	Sender   string
}

type ReportAtUser struct {
	Nickname string `gorm:"column:nickname"`
	UserID   string `gorm:"column:user_id"`
	Count    int64  `gorm:"column:count"`
}

type nightRow struct {
	Time     time.Time `gorm:"column:time"`
	Card     string    `gorm:"column:card"`
	SenderID string    `gorm:"column:sender_id"`
	Nickname string    `gorm:"column:nickname"`
}

func reverseStoredMessages(items []StoredMessage) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}
