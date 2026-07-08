package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"njk_go/internal/client/pgstore"
	"njk_go/internal/dal/model"

	"github.com/pgvector/pgvector-go"
)

const (
	memoryFactTopK                  = 5
	memoryImpressionTopK            = 2
	memoryFactMinSimilarity         = 0.35
	memoryImpressionMinSimilarity   = 0.35
	memoryFactDedupSimilarity       = 0.92
	memoryImpressionDedupSimilarity = 0.88
)

type memorySource struct {
	GroupID     string
	UserID      string
	MessageID   string
	Content     string
	ContextText string
	ActorName   string
	IsBotReply  bool
	QueuedAt    time.Time
}

type memoryExtractionResult struct {
	Memories []memoryCandidate `json:"memories"`
}

type memoryCandidate struct {
	Type       string  `json:"type"`
	UserID     string  `json:"user_id"`
	Content    string  `json:"content"`
	Confidence float32 `json:"confidence"`
}

func (s *Service) rememberIncomingMessage(ctx context.Context, event *memorySource) {
	s.enqueueMemorySource(event)
}

func (s *Service) rememberBotReply(ctx context.Context, source *memorySource) {
	s.enqueueMemorySource(source)
}

func (s *Service) processMemoryBatch(ctx context.Context, batch pendingMemoryBatch) {
	if s == nil || s.store == nil || s.freeAIClient == nil || s.embedClient == nil {
		log.Printf("【记忆批处理跳过】bucket=%s reason=dependency_nil", batch.Key)
		return
	}
	if len(batch.Items) == 0 {
		log.Printf("【记忆批处理跳过】bucket=%s reason=empty_batch", batch.Key)
		return
	}
	source := aggregateMemorySources(batch.Items)
	log.Printf("【记忆批处理开始】reason=%s bucket=%s size=%d group=%s user=%s message=%s bot_reply=%t",
		batch.Reason, batch.Key, len(batch.Items), source.GroupID, source.UserID, source.MessageID, source.IsBotReply)

	content := strings.TrimSpace(source.Content)
	if shouldSkipMemoryWrite(content) {
		log.Printf("【记忆批处理跳过】group=%s user=%s message=%s reason=content_filter content=%q",
			source.GroupID, source.UserID, source.MessageID, content)
		return
	}

	candidates, err := s.extractMemoryCandidates(ctx, source)
	if err != nil {
		log.Printf("【记忆提取失败】group=%s message=%s err=%v", source.GroupID, source.MessageID, err)
		return
	}
	if len(candidates) == 0 {
		log.Printf("【记忆提取为空】group=%s message=%s", source.GroupID, source.MessageID)
		return
	}
	log.Printf("【记忆提取成功】group=%s message=%s candidates=%d", source.GroupID, source.MessageID, len(candidates))

	texts := make([]string, 0, len(candidates))
	filtered := make([]memoryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Content = strings.TrimSpace(candidate.Content)
		if candidate.Content == "" {
			continue
		}
		if candidate.Type == "impression" && candidate.UserID == "" && source.UserID != "" {
			candidate.UserID = source.UserID
		}
		if candidate.Type != "fact" && candidate.Type != "impression" {
			continue
		}
		texts = append(texts, candidate.Content)
		filtered = append(filtered, candidate)
	}
	if len(filtered) == 0 {
		log.Printf("【记忆候选过滤后为空】group=%s message=%s", source.GroupID, source.MessageID)
		return
	}

	log.Printf("【记忆向量生成开始】group=%s message=%s candidates=%d", source.GroupID, source.MessageID, len(filtered))
	embeddings, err := s.embedClient.EmbedBatch(ctx, texts)
	if err != nil {
		log.Printf("【记忆向量生成失败】group=%s message=%s err=%v", source.GroupID, source.MessageID, err)
		return
	}
	if len(embeddings) != len(filtered) {
		log.Printf("【记忆向量数量不匹配】group=%s message=%s want=%d got=%d", source.GroupID, source.MessageID, len(filtered), len(embeddings))
		return
	}
	log.Printf("【记忆向量生成成功】group=%s message=%s vectors=%d", source.GroupID, source.MessageID, len(embeddings))

	for i, candidate := range filtered {
		vector := pgvector.NewVector(embeddings[i])
		hash := hashMemoryContent(candidate.Content)
		log.Printf("【记忆候选处理】group=%s message=%s type=%s user=%s hash=%s confidence=%.2f content=%q",
			source.GroupID, source.MessageID, candidate.Type, candidate.UserID, hash, candidate.Confidence, candidate.Content)
		switch candidate.Type {
		case "fact":
			if err := s.saveFactMemory(ctx, source, candidate, hash, vector); err != nil {
				log.Printf("【保存事实记忆失败】group=%s message=%s err=%v", source.GroupID, source.MessageID, err)
			}
		case "impression":
			if err := s.saveImpressionMemory(ctx, source, candidate, hash, vector); err != nil {
				log.Printf("【保存人物印象失败】group=%s message=%s err=%v", source.GroupID, source.MessageID, err)
			}
		}
	}
}

func (s *Service) extractMemoryCandidates(ctx context.Context, source memorySource) ([]memoryCandidate, error) {
	if s == nil || s.freeAIClient == nil {
		return nil, nil
	}

	recentText := strings.TrimSpace(source.ContextText)
	if recentText == "" {
		recent, err := s.store.RecentMessages(ctx, source.GroupID, 6)
		if err != nil {
			return nil, err
		}
		recentText = strings.Join(formatStoredMessages(recent), "\n")
	}
	userPrompt := fmt.Sprintf("当前消息来源信息：\n- group_id: %s\n- user_id: %s\n- actor_name: %s\n- is_bot_reply: %t\n- message_id: %s\n\n当前消息内容：\n%s\n\n最近消息窗口：\n%s\n\n请输出 JSON。",
		source.GroupID,
		source.UserID,
		source.ActorName,
		source.IsBotReply,
		source.MessageID,
		source.Content,
		recentText,
	)
	result, err := s.freeAIClient.Complete(ctx, memoryExtractionSystemPrompt, userPrompt, nil)
	if err != nil {
		return nil, err
	}

	result = trimJSONCodeFence(result)
	var parsed memoryExtractionResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		return nil, err
	}
	return parsed.Memories, nil
}

func (s *Service) saveFactMemory(ctx context.Context, source memorySource, candidate memoryCandidate, contentHash string, vector pgvector.Vector) error {
	var userID *string
	if candidate.UserID != "" {
		userID = &candidate.UserID
	}
	exists, err := s.store.MemoryFactExistsByHash(ctx, source.GroupID, userID, contentHash)
	if err != nil {
		return err
	}
	if exists {
		log.Printf("【事实记忆跳过】group=%s user=%s message=%s reason=hash_exists hash=%s",
			source.GroupID, nullableString(userID), source.MessageID, contentHash)
		return nil
	}
	similar, err := s.store.FindNearestFactMemory(ctx, source.GroupID, userID, vector)
	if err != nil {
		return err
	}
	if similar != nil && similar.Similarity >= memoryFactDedupSimilarity {
		log.Printf("【事实记忆跳过】group=%s user=%s message=%s reason=similarity_dedup similarity=%.4f threshold=%.2f",
			source.GroupID, nullableString(userID), source.MessageID, similar.Similarity, memoryFactDedupSimilarity)
		return nil
	}
	now := time.Now()
	var messageID *string
	if source.MessageID != "" {
		messageID = &source.MessageID
	}
	record := &model.MemoryFact{
		GroupID:     source.GroupID,
		UserID:      userID,
		MessageID:   messageID,
		Content:     candidate.Content,
		ContentHash: contentHash,
		Embedding:   vector,
		Confidence:  candidate.Confidence,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.SaveMemoryFact(ctx, record); err != nil {
		return err
	}
	log.Printf("【保存事实记忆成功】group=%s user=%s message=%s confidence=%.2f content=%q",
		source.GroupID,
		nullableString(userID),
		source.MessageID,
		candidate.Confidence,
		candidate.Content,
	)
	return nil
}

func (s *Service) saveImpressionMemory(ctx context.Context, source memorySource, candidate memoryCandidate, contentHash string, vector pgvector.Vector) error {
	userID := strings.TrimSpace(candidate.UserID)
	if userID == "" {
		log.Printf("【人物印象跳过】group=%s message=%s reason=empty_user_id", source.GroupID, source.MessageID)
		return nil
	}
	exists, err := s.store.MemoryImpressionExistsByHash(ctx, source.GroupID, userID, contentHash)
	if err != nil {
		return err
	}
	if exists {
		log.Printf("【人物印象跳过】group=%s user=%s message=%s reason=hash_exists hash=%s",
			source.GroupID, userID, source.MessageID, contentHash)
		return nil
	}
	nextVersion, err := s.store.LatestImpressionVersion(ctx, source.GroupID, userID)
	if err != nil {
		return err
	}
	similar, err := s.store.FindNearestImpression(ctx, source.GroupID, userID, vector)
	if err != nil {
		return err
	}
	if similar != nil && similar.Similarity >= memoryImpressionDedupSimilarity {
		log.Printf("【人物印象去重替换】group=%s user=%s message=%s old_id=%d similarity=%.4f threshold=%.2f",
			source.GroupID, userID, source.MessageID, similar.ID, similar.Similarity, memoryImpressionDedupSimilarity)
		if err := s.store.DeactivateMemoryImpression(ctx, similar.ID); err != nil {
			return err
		}
	}
	now := time.Now()
	var messageID *string
	if source.MessageID != "" {
		messageID = &source.MessageID
	}
	record := &model.MemoryImpression{
		GroupID:     source.GroupID,
		UserID:      userID,
		MessageID:   messageID,
		Content:     candidate.Content,
		ContentHash: contentHash,
		Embedding:   vector,
		Confidence:  candidate.Confidence,
		Version:     nextVersion + 1,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.SaveMemoryImpression(ctx, record); err != nil {
		return err
	}
	log.Printf("【保存人物印象成功】group=%s user=%s message=%s version=%d confidence=%.2f content=%q",
		source.GroupID,
		userID,
		source.MessageID,
		record.Version,
		candidate.Confidence,
		candidate.Content,
	)
	return nil
}

func (s *Service) BuildMemoryContext(ctx context.Context, groupID string, userID string, queryText string) (string, error) {
	if s == nil || s.store == nil || s.embedClient == nil {
		log.Printf("【记忆检索跳过】group=%s user=%s reason=dependency_nil", groupID, userID)
		return "", nil
	}
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		log.Printf("【记忆检索跳过】group=%s user=%s reason=empty_query", groupID, userID)
		return "", nil
	}

	embedding, err := s.embedClient.Embed(ctx, queryText)
	if err != nil {
		return "", err
	}
	log.Printf("【记忆查询向量生成成功】group=%s user=%s query=%q", groupID, userID, queryText)
	vector := pgvector.NewVector(embedding)

	facts := make([]pgstore.MemorySearchHit, 0, memoryFactTopK)
	if userID != "" {
		personalFacts, err := s.store.SearchFactMemories(ctx, groupID, &userID, vector, memoryFactTopK, memoryFactMinSimilarity)
		if err != nil {
			return "", err
		}
		facts = append(facts, personalFacts...)
	}
	groupFacts, err := s.store.SearchFactMemories(ctx, groupID, nil, vector, memoryFactTopK, memoryFactMinSimilarity)
	if err != nil {
		return "", err
	}
	facts = append(facts, groupFacts...)
	if len(facts) > memoryFactTopK {
		facts = facts[:memoryFactTopK]
	}

	impressions := []pgstore.MemorySearchHit{}
	if userID != "" {
		impressions, err = s.store.SearchImpressionMemories(ctx, groupID, userID, vector, memoryImpressionTopK, memoryImpressionMinSimilarity)
		if err != nil {
			return "", err
		}
	}
	block := formatMemoryContext(facts, impressions)
	if block != "" {
		log.Printf("【记忆检索成功】group=%s user=%s facts=%d impressions=%d query=%q recalled=%s",
			groupID,
			userID,
			len(facts),
			len(impressions),
			queryText,
			block,
		)
	} else {
		log.Printf("【记忆检索为空】group=%s user=%s query=%q", groupID, userID, queryText)
	}
	return block, nil
}

func (s *Service) enrichPromptWithMemory(ctx context.Context, groupID string, userID string, queryText string, basePrompt string) string {
	block, err := s.BuildMemoryContext(ctx, groupID, userID, queryText)
	if err != nil {
		log.Printf("【记忆检索失败】group=%s user=%s err=%v", groupID, userID, err)
		return basePrompt
	}
	if block == "" {
		return basePrompt
	}
	return "以下是可能相关的长期记忆，仅在相关时参考；若与当前聊天冲突，以当前聊天为准。\n" + block + "\n\n当前输入：\n" + basePrompt
}

func shouldSkipMemoryWrite(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return true
	}
	if utf8.RuneCountInString(content) < 6 {
		return true
	}
	if strings.HasPrefix(content, ".") {
		return true
	}
	return false
}

func formatMemoryContext(facts []pgstore.MemorySearchHit, impressions []pgstore.MemorySearchHit) string {
	parts := make([]string, 0, 2)
	if len(facts) > 0 {
		lines := make([]string, 0, len(facts))
		for _, item := range facts {
			lines = append(lines, fmt.Sprintf("- %s", item.Content))
		}
		parts = append(parts, "【长期事实】\n"+strings.Join(lines, "\n"))
	}
	if len(impressions) > 0 {
		lines := make([]string, 0, len(impressions))
		for _, item := range impressions {
			lines = append(lines, fmt.Sprintf("- %s", item.Content))
		}
		parts = append(parts, "【人物印象】\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

func hashMemoryContent(content string) string {
	sum := md5.Sum([]byte(content))
	return hex.EncodeToString(sum[:])
}

func trimJSONCodeFence(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func aggregateMemorySources(items []memorySource) memorySource {
	if len(items) == 0 {
		return memorySource{}
	}
	last := items[len(items)-1]
	sameUser := true
	firstUserID := strings.TrimSpace(items[0].UserID)
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.UserID) != firstUserID {
			sameUser = false
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		actor := strings.TrimSpace(item.ActorName)
		if actor == "" {
			actor = strings.TrimSpace(item.UserID)
		}
		userID := strings.TrimSpace(item.UserID)
		switch {
		case actor != "" && userID != "":
			parts = append(parts, fmt.Sprintf("%s(%s): %s", actor, userID, content))
		case actor != "":
			parts = append(parts, fmt.Sprintf("%s: %s", actor, content))
		default:
			parts = append(parts, content)
		}
	}
	if !sameUser {
		last.UserID = ""
	}
	last.Content = strings.Join(parts, "\n")
	last.ContextText = last.Content
	return last
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

var memoryExtractionSystemPrompt = strings.TrimSpace(`
你是一个群聊长期记忆提取器。你的任务是从当前消息和最近少量上下文中，只提取“未来仍有价值”的长期记忆。

输出要求：
1. 只能输出 JSON。
2. JSON 格式固定为 {"memories":[...]}。
3. 每个 memory 对象字段固定为：
   - type: "fact" 或 "impression"
   - user_id: 关联用户；群公共事实可为空字符串
   - content: 精炼后的中文记忆文本
   - confidence: 0 到 1 的小数

提取原则：
1. fact 只保留稳定事实、偏好、关系、长期约定、身份信息、持续项目状态。
2. impression 只保留对某个用户的长期印象、偏好、风格、性格倾向。
3. 不要提取临时寒暄、一次性情绪、无意义口头禅、纯命令、纯噪声。
4. 如果没有值得长期保存的内容，返回 {"memories":[]}。
5. content 要尽量短、明确、可复用，不要照抄大段原文。
`)
