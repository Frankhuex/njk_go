package service

import (
	"context"
	"fmt"
	"time"

	"njk_go/internal/dal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	db *gorm.DB
}

type GetFaceIDMessageRow struct {
	MessageID        string
	SegmentFaceIDs   []string
	EmojiLikeFaceIDs []string
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

func (s *Store) UpsertFace(ctx context.Context, faceID string) error {
	if faceID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Face{
		FaceID: faceID,
	}).Error
}

func (s *Store) SaveEmojiLike(ctx context.Context, messageID string, userID string, faceID string) error {
	if messageID == "" || userID == "" || faceID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Face{
			FaceID: faceID,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.EmojiLike{
			MessageID: messageID,
			UserID:    userID,
			FaceID:    faceID,
		}).Error
	})
}

func (s *Store) EnsureNoticeMessage(ctx context.Context, messageID string, groupID string, userID string, eventTime int64) error {
	if messageID == "" {
		return nil
	}
	messageTime := time.Now()
	if eventTime > 0 {
		messageTime = time.Unix(eventTime, 0)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if userID != "" {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}},
				DoNothing: true,
			}).Create(&model.User{
				UserID:   userID,
				Nickname: "",
			}).Error; err != nil {
				return err
			}
		}
		if groupID != "" {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "group_id"}},
				DoNothing: true,
			}).Create(&model.Group{
				GroupID:   groupID,
				GroupName: "",
			}).Error; err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Message{
			MessageID: messageID,
			Time:      messageTime,
			SenderID:  nullableString(userID),
			GroupID:   nullableString(groupID),
		}).Error
	})
}

func (s *Store) SaveImage(ctx context.Context, messageID string, imageHash string, imageURL string) (*model.Image, error) {
	record := &model.Image{
		MessageID: messageID,
		ImageHash: imageHash,
		URL:       nullableString(imageURL),
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
	if limit <= 0 {
		return nil, nil
	}
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

func (s *Store) RecentMessageImages(ctx context.Context, groupID string, limit int) ([]model.Image, error) {
	if limit <= 0 {
		return nil, nil
	}

	var rows []model.Image
	recentMessageIDs := s.db.WithContext(ctx).
		Table(`message AS m`).
		Select(`m.message_id`).
		Where("m.group_id = ?", groupID).
		Order("m.time DESC").
		Limit(limit)

	err := s.db.WithContext(ctx).
		Table(`image AS i`).
		Select(`i.id, i.message_id, i.image_hash, i.url`).
		Joins(`JOIN message m ON i.message_id = m.message_id`).
		Where("i.message_id IN (?)", recentMessageIDs).
		Order("m.time ASC, i.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) RecentFaceIDRows(ctx context.Context, groupID string, limit int) ([]GetFaceIDMessageRow, error) {
	if limit <= 0 {
		return nil, nil
	}

	type recentMessageRow struct {
		MessageID string `gorm:"column:message_id"`
		RawJSON   string `gorm:"column:raw_json"`
	}
	var recentMessages []recentMessageRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT recent.message_id, recent.raw_json
		FROM (
			SELECT m.message_id, m."time", COALESCE(m.raw_json::text, '') AS raw_json
			FROM message AS m
			WHERE m.group_id = ?
			ORDER BY m."time" DESC
			LIMIT ?
		) AS recent
		ORDER BY recent."time" ASC
	`, groupID, limit).Scan(&recentMessages).Error; err != nil {
		return nil, err
	}
	if len(recentMessages) == 0 {
		return nil, nil
	}

	result := make([]GetFaceIDMessageRow, 0, len(recentMessages))
	rowByMessageID := make(map[string]*GetFaceIDMessageRow, len(recentMessages))
	for _, recent := range recentMessages {
		row := GetFaceIDMessageRow{MessageID: recent.MessageID}
		if faceIDs, err := extractFaceIDsFromRawJSON(recent.RawJSON); err == nil {
			row.SegmentFaceIDs = faceIDs
			sortFaceIDs(row.SegmentFaceIDs)
		}
		result = append(result, row)
		rowByMessageID[recent.MessageID] = &result[len(result)-1]
	}

	type emojiLikeFaceIDRow struct {
		MessageID string `gorm:"column:message_id"`
		FaceID    string `gorm:"column:face_id"`
	}
	var likeRows []emojiLikeFaceIDRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT recent.message_id, el.face_id
		FROM emoji_like AS el
		INNER JOIN (
			SELECT m.message_id, m."time"
			FROM message AS m
			WHERE m.group_id = ?
			ORDER BY m."time" DESC
			LIMIT ?
		) AS recent ON recent.message_id = el.message_id
		ORDER BY
			recent."time" ASC,
			CASE WHEN el.face_id ~ '^\d+$' THEN el.face_id::bigint END ASC NULLS LAST,
			el.face_id ASC
	`, groupID, limit).Scan(&likeRows).Error; err != nil {
		return nil, err
	}
	for _, likeRow := range likeRows {
		row := rowByMessageID[likeRow.MessageID]
		if row == nil {
			continue
		}
		row.EmojiLikeFaceIDs = append(row.EmojiLikeFaceIDs, likeRow.FaceID)
	}
	for i := range result {
		sortFaceIDs(result[i].EmojiLikeFaceIDs)
	}
	return result, nil
}

func (s *Store) AllFaceIDs(ctx context.Context) ([]string, []string, error) {
	var allFaceIDs []string
	if err := s.db.WithContext(ctx).Raw(`SELECT face_id FROM face`).Scan(&allFaceIDs).Error; err != nil {
		return nil, nil, err
	}
	sortFaceIDs(allFaceIDs)

	var likedFaceIDs []string
	if err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT face_id FROM emoji_like`).Scan(&likedFaceIDs).Error; err != nil {
		return nil, nil, err
	}
	sortFaceIDs(likedFaceIDs)

	return allFaceIDs, likedFaceIDs, nil
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

func (s *Store) GroupImageCandidates(ctx context.Context, groupID string, excludeImageID int32, excludeMessageID string) ([]StoredImage, error) {
	var rows []StoredImage
	err := s.db.WithContext(ctx).
		Table(`image AS i`).
		Select(`i.id, i.message_id, i.image_hash, i.url, m.time, COALESCE(m.sender_id, '') AS sender_id, COALESCE(u.nickname, '') AS nickname, COALESCE(m.card, '') AS card`).
		Joins(`JOIN message m ON i.message_id = m.message_id`).
		Joins(`LEFT JOIN "user" u ON m.sender_id = u.user_id`).
		Where("m.group_id = ? AND i.id <> ? AND i.message_id <> ?", groupID, excludeImageID, excludeMessageID).
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

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (m StoredMessage) Format() string {
	name := m.Card
	if name == "" {
		name = m.Nickname
	}
	if name == "" {
		name = "Unknown User"
	}
	return fmt.Sprintf("[%s,消息%s] %s (qq号%s): %s", formatDisplayTime(m.Time), m.MessageID, name, m.SenderID, m.Text)
}

type StoredImage struct {
	ID        int32     `gorm:"column:id"`
	MessageID string    `gorm:"column:message_id"`
	ImageHash string    `gorm:"column:image_hash"`
	URL       *string   `gorm:"column:url"`
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
