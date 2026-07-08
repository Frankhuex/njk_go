package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"njk_go/internal/client/pgstore"
)

const (
	memoryBackfillTaskName     = "memory_factory"
	memoryBackfillStatePath    = "runtime/memory-factory/state.json"
	memoryBackfillHistoryDir   = "runtime/memory-factory/history"
	memoryBackfillBatchLimit   = 100
	memoryBackfillTickInterval = time.Minute
)

type memoryBackfillState struct {
	TaskName  string                                  `json:"task_name"`
	StartedAt time.Time                               `json:"started_at"`
	RunEndAt  time.Time                               `json:"run_end_at"`
	Status    string                                  `json:"status"`
	Groups    map[string]*memoryBackfillGroupProgress `json:"groups"`
}

type memoryBackfillGroupProgress struct {
	Status            string     `json:"status"`
	LastMessageTime   *time.Time `json:"last_message_time,omitempty"`
	LastMessageID     string     `json:"last_message_id,omitempty"`
	ProcessedMessages int        `json:"processed_messages"`
	LastError         string     `json:"last_error,omitempty"`
}

type memoryBackfillStateStore struct {
	mu         sync.Mutex
	path       string
	historyDir string
	state      *memoryBackfillState
}

func (s *Service) RunMemoryBackfill(ctx context.Context) error {
	if s == nil || s.store == nil || s.freeAIClient == nil || s.embedClient == nil {
		return fmt.Errorf("memory backfill dependencies not available")
	}

	stateStore, loaded, err := newMemoryBackfillStateStore(ctx, s.store, memoryBackfillStatePath, memoryBackfillHistoryDir, time.Now())
	if err != nil {
		return err
	}
	activeGroupIDs, err := stateStore.PrepareActiveGroups(ctx, s.store)
	if err != nil {
		return err
	}
	if loaded {
		log.Printf("【JSON状态恢复】path=%s run_end_at=%s active_groups=%d", memoryBackfillStatePath, stateStore.RunEndAt().Format(time.RFC3339), len(activeGroupIDs))
	} else {
		log.Printf("【JSON状态初始化】path=%s run_end_at=%s groups=%d", memoryBackfillStatePath, stateStore.RunEndAt().Format(time.RFC3339), len(activeGroupIDs))
	}

	log.Printf("【记忆回填任务启动】task=%s run_end_at=%s groups=%d", memoryBackfillTaskName, stateStore.RunEndAt().Format(time.RFC3339), len(activeGroupIDs))
	if len(activeGroupIDs) == 0 {
		if err := stateStore.MarkTaskDone(false); err != nil {
			return err
		}
		log.Printf("【记忆回填任务结束】task=%s groups=0 reason=no_incremental_messages", memoryBackfillTaskName)
		return nil
	}

	var wg sync.WaitGroup
	for _, groupID := range activeGroupIDs {
		wg.Add(1)
		go func(groupID string) {
			defer wg.Done()
			s.runMemoryBackfillGroup(ctx, stateStore, groupID)
		}(groupID)
	}
	wg.Wait()

	if err := stateStore.MarkTaskDone(true); err != nil {
		return err
	}
	log.Printf("【记忆回填任务结束】task=%s groups=%d", memoryBackfillTaskName, len(activeGroupIDs))
	return nil
}

func (s *Service) runMemoryBackfillGroup(ctx context.Context, stateStore *memoryBackfillStateStore, groupID string) {
	log.Printf("【群回填启动】group=%s", groupID)
	if err := stateStore.MarkGroupRunning(groupID); err != nil {
		log.Printf("【群回填状态更新失败】group=%s err=%v", groupID, err)
		return
	}

	ticker := time.NewTicker(memoryBackfillTickInterval)
	defer ticker.Stop()

	for {
		done, err := s.processMemoryBackfillGroupTick(ctx, stateStore, groupID)
		if err != nil {
			log.Printf("【群回填失败】group=%s err=%v", groupID, err)
			if markErr := stateStore.MarkGroupError(groupID, err.Error()); markErr != nil {
				log.Printf("【JSON状态更新失败】group=%s err=%v", groupID, markErr)
			}
		} else if done {
			if err := stateStore.MarkGroupDone(groupID); err != nil {
				log.Printf("【群回填状态更新失败】group=%s err=%v", groupID, err)
			}
			log.Printf("【群回填完成】group=%s", groupID)
			return
		}

		log.Printf("【群回填等待下一轮】group=%s interval=%s", groupID, memoryBackfillTickInterval)
		select {
		case <-ctx.Done():
			log.Printf("【群回填停止】group=%s reason=context_done", groupID)
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) processMemoryBackfillGroupTick(ctx context.Context, stateStore *memoryBackfillStateStore, groupID string) (bool, error) {
	cursorTime, cursorMessageID := stateStore.Cursor(groupID)
	rows, err := s.store.HistoricalMessagesBatch(ctx, groupID, cursorTime, cursorMessageID, stateStore.RunEndAt(), memoryBackfillBatchLimit)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		log.Printf("【历史消息批读取为空】group=%s", groupID)
		return true, nil
	}

	log.Printf("【历史消息批读取成功】group=%s count=%d run_end_at=%s", groupID, len(rows), stateStore.RunEndAt().Format(time.RFC3339))
	batches := buildBackfillMemoryBatches(groupID, rows, s.cfg.BotUserID)
	for _, batch := range batches {
		log.Printf("【历史窗口生产开始】group=%s reason=%s size=%d", groupID, batch.Reason, len(batch.Items))
		s.processMemoryBatch(ctx, batch)
		log.Printf("【历史窗口生产完成】group=%s reason=%s size=%d", groupID, batch.Reason, len(batch.Items))
	}

	last := rows[len(rows)-1]
	if err := stateStore.AdvanceGroup(groupID, last, len(rows)); err != nil {
		return false, err
	}

	if len(rows) < memoryBackfillBatchLimit {
		return true, nil
	}
	return false, nil
}

func buildBackfillMemoryBatches(groupID string, rows []pgstore.StoredMessage, botUserID string) []pendingMemoryBatch {
	sources := make([]memorySource, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, memorySource{
			GroupID:    groupID,
			UserID:     row.SenderID,
			MessageID:  row.MessageID,
			Content:    firstNonEmpty(row.RawMessage, row.Text),
			ActorName:  firstNonEmpty(row.Card, row.Nickname, row.SenderID),
			IsBotReply: row.SenderID == botUserID,
		})
	}

	batches := make([]pendingMemoryBatch, 0, (len(sources)+memoryBatchSize-1)/memoryBatchSize)
	for start := 0; start < len(sources); start += memoryBatchSize {
		end := start + memoryBatchSize
		reason := "backfill_count"
		if end > len(sources) {
			end = len(sources)
			reason = "backfill_flush"
		}
		chunk := append([]memorySource(nil), sources[start:end]...)
		batches = append(batches, pendingMemoryBatch{
			Key:    groupID,
			Items:  chunk,
			Reason: reason,
		})
	}
	return batches
}

func newMemoryBackfillStateStore(ctx context.Context, store *pgstore.Store, path string, historyDir string, now time.Time) (*memoryBackfillStateStore, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		return nil, false, err
	}

	var state *memoryBackfillState
	loaded := false
	if data, err := os.ReadFile(path); err == nil {
		var parsed memoryBackfillState
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, false, err
		}
		state = &parsed
		loaded = true
	} else if !os.IsNotExist(err) {
		return nil, false, err
	}

	groupIDs, err := store.GroupIDsWithMessages(ctx)
	if err != nil {
		return nil, false, err
	}
	if state == nil {
		state = &memoryBackfillState{
			TaskName: memoryBackfillTaskName,
			Groups:   map[string]*memoryBackfillGroupProgress{},
		}
	}
	if state.Groups == nil {
		state.Groups = map[string]*memoryBackfillGroupProgress{}
	}
	for _, groupID := range groupIDs {
		if state.Groups[groupID] == nil {
			state.Groups[groupID] = &memoryBackfillGroupProgress{Status: "pending"}
		}
	}
	state.TaskName = memoryBackfillTaskName
	state.StartedAt = now
	state.RunEndAt = now
	state.Status = "running"
	manager := &memoryBackfillStateStore{
		path:       path,
		historyDir: historyDir,
		state:      state,
	}
	if err := manager.persistLocked(); err != nil {
		return nil, false, err
	}
	return manager, loaded, nil
}

func (m *memoryBackfillStateStore) RunEndAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.RunEndAt
}

func (m *memoryBackfillStateStore) GroupIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return sortedGroupIDs(m.state.Groups, nil)
}

func (m *memoryBackfillStateStore) ActiveGroupIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return sortedGroupIDs(m.state.Groups, func(progress *memoryBackfillGroupProgress) bool {
		return progress == nil || progress.Status != "done"
	})
}

func (m *memoryBackfillStateStore) PrepareActiveGroups(ctx context.Context, store *pgstore.Store) ([]string, error) {
	m.mu.Lock()
	runEndAt := m.state.RunEndAt
	groupIDs := sortedGroupIDs(m.state.Groups, nil)
	cursors := make(map[string]memoryBackfillGroupProgress, len(groupIDs))
	for _, groupID := range groupIDs {
		if progress := m.state.Groups[groupID]; progress != nil {
			cursors[groupID] = *progress
		} else {
			cursors[groupID] = memoryBackfillGroupProgress{}
		}
	}
	m.mu.Unlock()

	active := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		progress := cursors[groupID]
		rows, err := store.HistoricalMessagesBatch(ctx, groupID, progress.LastMessageTime, progress.LastMessageID, runEndAt, 1)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			active = append(active, groupID)
			if err := m.setGroupStatus(groupID, "pending", ""); err != nil {
				return nil, err
			}
			continue
		}
		if err := m.setGroupStatus(groupID, "done", ""); err != nil {
			return nil, err
		}
	}
	return active, nil
}

func sortedGroupIDs(groups map[string]*memoryBackfillGroupProgress, keep func(progress *memoryBackfillGroupProgress) bool) []string {
	groupIDs := make([]string, 0, len(groups))
	for groupID, progress := range groups {
		if keep != nil && !keep(progress) {
			continue
		}
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	return groupIDs
}

func (m *memoryBackfillStateStore) Cursor(groupID string) (*time.Time, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	group := m.state.Groups[groupID]
	if group == nil {
		return nil, ""
	}
	if group.LastMessageTime == nil || group.LastMessageTime.IsZero() {
		return nil, ""
	}
	value := *group.LastMessageTime
	return &value, group.LastMessageID
}

func (m *memoryBackfillStateStore) MarkGroupRunning(groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	group := m.ensureGroupLocked(groupID)
	group.Status = "running"
	group.LastError = ""
	return m.persistLocked()
}

func (m *memoryBackfillStateStore) setGroupStatus(groupID string, status string, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	group := m.ensureGroupLocked(groupID)
	group.Status = status
	group.LastError = lastError
	return m.persistLocked()
}

func (m *memoryBackfillStateStore) MarkGroupError(groupID string, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	group := m.ensureGroupLocked(groupID)
	group.Status = "running"
	group.LastError = lastError
	return m.persistLocked()
}

func (m *memoryBackfillStateStore) AdvanceGroup(groupID string, last pgstore.StoredMessage, processedCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	group := m.ensureGroupLocked(groupID)
	lastTime := last.Time
	group.Status = "running"
	group.LastMessageTime = &lastTime
	group.LastMessageID = last.MessageID
	group.ProcessedMessages += processedCount
	group.LastError = ""
	if err := m.persistLocked(); err != nil {
		return err
	}
	log.Printf("【JSON状态更新】group=%s last_message_time=%s last_message_id=%s processed_messages=%d",
		groupID, lastTime.Format(time.RFC3339), last.MessageID, group.ProcessedMessages)
	return nil
}

func (m *memoryBackfillStateStore) MarkGroupDone(groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	group := m.ensureGroupLocked(groupID)
	group.Status = "done"
	group.LastError = ""
	return m.persistLocked()
}

func (m *memoryBackfillStateStore) MarkTaskDone(archive bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Status = "done"
	if err := m.persistLocked(); err != nil {
		return err
	}
	if !archive {
		return nil
	}

	archivePath := filepath.Join(m.historyDir, fmt.Sprintf("%s_%s.json", m.state.TaskName, m.state.StartedAt.Format("20060102_150405")))
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(archivePath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	log.Printf("【JSON状态归档】path=%s", archivePath)
	return nil
}

func (m *memoryBackfillStateStore) ensureGroupLocked(groupID string) *memoryBackfillGroupProgress {
	if m.state.Groups == nil {
		m.state.Groups = map[string]*memoryBackfillGroupProgress{}
	}
	group := m.state.Groups[groupID]
	if group == nil {
		group = &memoryBackfillGroupProgress{Status: "pending"}
		m.state.Groups[groupID] = group
	}
	return group
}

func (m *memoryBackfillStateStore) persistLocked() error {
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := m.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, m.path)
}
