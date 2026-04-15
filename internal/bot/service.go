package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"njk_go/internal/bbh"
	"njk_go/internal/config"
	"njk_go/internal/model"
	"njk_go/internal/napcat"

	"gorm.io/gorm"
)

type AICompleter interface {
	Complete(ctx context.Context, systemPrompt string, userPrompt string, temperature *float64) (string, error)
}

type outboundWriter interface {
	WriteText(payload []byte) error
}

type Service struct {
	cfg          config.Config
	store        *Store
	aiClient     AICompleter
	freeAIClient AICompleter
	bbhClient    *bbh.Client
	imageService *ImageService
	patterns     []*regexp.Regexp
	rng          *rand.Rand
	pending      *pendingQueue
	lastAIMu     sync.Mutex
	lastAI       map[string]time.Time
}

func NewService(cfg config.Config, db *gorm.DB, aiClient AICompleter, freeAIClient AICompleter, bbhClient *bbh.Client) *Service {
	patterns := make([]*regexp.Regexp, 0, len(patternSources(cfg.BotUserID)))
	for _, source := range patternSources(cfg.BotUserID) {
		patterns = append(patterns, regexp.MustCompile(source))
	}

	store := NewStore(db)
	return &Service{
		cfg:          cfg,
		store:        store,
		aiClient:     aiClient,
		freeAIClient: freeAIClient,
		bbhClient:    bbhClient,
		imageService: NewImageService(store),
		patterns:     patterns,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
		pending:      &pendingQueue{},
		lastAI:       map[string]time.Time{},
	}
}

func (s *Service) HandleNotice(ctx context.Context, conn outboundWriter, clientAddr string, event *napcat.NoticeEvent) {
	if event == nil {
		return
	}

	log.Printf("【处理Notice】%s - 群ID: %s target_id=%s self_id=%s", clientAddr, event.GroupID, event.TargetID, event.SelfID)

	if event.TargetID == "" || event.SelfID == "" || event.TargetID != event.SelfID {
		return
	}

	if err := s.sendGroupText(ctx, conn, event.GroupID.String(), "灰色中分已然绽放", false); err != nil {
		log.Printf("【发送Notice响应失败】%s - %v", clientAddr, err)
		return
	}

	log.Printf("【发送Notice响应】%s - 群ID: %s", clientAddr, event.GroupID)
}

func (s *Service) HandleGroupMessage(ctx context.Context, conn outboundWriter, clientAddr string, event *napcat.GroupMessageEvent) {
	if event == nil {
		return
	}

	groupID := event.GroupID.String()
	if len(s.cfg.AllowedGroupIDs) > 0 {
		if _, ok := s.cfg.AllowedGroupIDs[groupID]; !ok {
			log.Printf("【忽略群消息】%s - 群:%s 不在白名单", clientAddr, groupID)
			return
		}
	}

	rawMessage := event.RawMessage
	match, index := s.matchIndex(rawMessage)
	if match == nil && mentionsBot(event.Message, s.cfg.BotUserID) {
		match = s.patterns[njkIndex]
		index = njkIndex
	}
	log.Printf("【处理群消息】%s - 群:%s 消息:%s 命中:%d", clientAddr, groupID, rawMessage, index)

	responses := []pendingOutbound{}

	if match == nil || index == njkIndex {
		duplicates, err := s.saveIncomingMessageAndCheckImages(ctx, event)
		if err != nil {
			log.Printf("【消息落库失败】%s - %v", clientAddr, err)
		}
		for _, duplicate := range duplicates {
			text := fmt.Sprintf("[CQ:reply,id=%s]🇫🇷%d遍了。%s在%s就🇫🇷了。", duplicate.MessageID, duplicate.Count, duplicate.SenderName, formatDisplayTime(duplicate.SentAt))
			responses = append(responses, pendingOutbound{
				GroupID:    groupID,
				Message:    text,
				ShouldSave: false,
			})
		}
	}

	if match != nil {
		outbound, err := s.handleMatchedCommand(ctx, event, match, index)
		if err != nil {
			log.Printf("【命令处理失败】%s - %v", clientAddr, err)
		} else if outbound != nil {
			responses = append(responses, *outbound)
		}
	} else if s.rng.Float64() < 0.08 {
		outbound, err := s.handleNJKReply(ctx, event, groupID)
		if err != nil {
			log.Printf("【随机发言失败】%s - %v", clientAddr, err)
		} else if outbound != nil {
			responses = append(responses, *outbound)
		}
	}

	for _, response := range responses {
		if err := s.sendGroupText(ctx, conn, response.GroupID, response.Message, response.ShouldSave); err != nil {
			log.Printf("【发送响应失败】%s - %v", clientAddr, err)
		}
	}
}

func (s *Service) HandleActionResponse(ctx context.Context, action *napcat.ActionEnvelope) {
	if action == nil || action.Status != "ok" || action.Retcode != 0 {
		return
	}

	var data napcat.SendMsgResponseData
	if err := json.Unmarshal(action.Data, &data); err != nil {
		return
	}
	if data.MessageID == "" {
		return
	}

	pending := s.pending.Pop()
	if pending == nil || !pending.ShouldSave {
		return
	}

	if err := s.saveSelfMessage(ctx, pending, data.MessageID.String()); err != nil {
		log.Printf("【保存自己消息失败】message_id=%s err=%v", data.MessageID, err)
		return
	}
	log.Printf("【完成自己消息存储】消息ID: %s", data.MessageID)
}

func (s *Service) sendGroupText(ctx context.Context, conn outboundWriter, groupID string, message string, shouldSave bool) error {
	message = normalizeOutboundText(message)
	req := napcat.SendGroupMsgRequest{
		Action: "send_group_msg",
		Params: napcat.SendGroupMsgParams{
			GroupID: napcat.ID(groupID),
			Message: napcat.NewTextMessage(message),
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := conn.WriteText(data); err != nil {
		return err
	}
	s.pending.Push(pendingMessage{
		GroupID:    groupID,
		Message:    message,
		SentAt:     time.Now(),
		ShouldSave: shouldSave,
	})
	log.Printf("【发送群消息】group=%s should_save=%t message=%s", groupID, shouldSave, message)
	return nil
}

func (s *Service) matchIndex(rawMessage string) (*regexp.Regexp, int) {
	for i := len(s.patterns) - 1; i >= 0; i-- {
		if s.patterns[i].MatchString(rawMessage) {
			return s.patterns[i], i
		}
	}
	return nil, -1
}

func (s *Service) handleMatchedCommand(ctx context.Context, event *napcat.GroupMessageEvent, pattern *regexp.Regexp, index int) (*pendingOutbound, error) {
	match := pattern.FindStringSubmatch(event.RawMessage)
	if match == nil {
		return nil, nil
	}

	groupID := event.GroupID.String()
	switch {
	case index < aiIndex:
		count, _ := strconv.Atoi(match[1])
		history, err := s.historyStrings(ctx, groupID, count)
		if err != nil {
			return nil, err
		}
		if len(history) == 0 {
			return &pendingOutbound{GroupID: groupID, Message: "历史消息不足", ShouldSave: false}, nil
		}
		result, err := s.aiClient.Complete(ctx, prompts[index], fmt.Sprintf("%v", history), nil)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: false}, nil
	case index == aiIndex:
		count, _ := strconv.Atoi(match[1])
		history, err := s.store.RecentMessages(ctx, groupID, count)
		if err != nil {
			return nil, err
		}
		if len(history) == 0 {
			return &pendingOutbound{GroupID: groupID, Message: "历史消息不足", ShouldSave: false}, nil
		}
		msgStrs := formatStoredMessages(history)
		result, err := s.aiClient.Complete(ctx, prompts[aiIndex], fmt.Sprintf("%v", msgStrs), nil)
		if err != nil {
			return nil, err
		}
		s.setLastAI(groupID, history[0].Time)
		return &pendingOutbound{GroupID: groupID, Message: fmt.Sprintf("[CQ:reply,id=%s]%s", history[0].MessageID, result), ShouldSave: true}, nil
	case index == njkIndex:
		return s.handleNJKReply(ctx, event, groupID)
	case index == aicIndex:
		start, ok := s.getLastAI(groupID)
		if !ok {
			return &pendingOutbound{GroupID: groupID, Message: "请先发起一次「.ai后接数字」", ShouldSave: false}, nil
		}
		history, err := s.store.MessagesSince(ctx, groupID, start)
		if err != nil {
			return nil, err
		}
		if len(history) == 0 {
			return &pendingOutbound{GroupID: groupID, Message: "历史消息不足", ShouldSave: false}, nil
		}
		msgStrs := formatStoredMessages(history)
		result, err := s.aiClient.Complete(ctx, prompts[aiIndex], fmt.Sprintf("%v", msgStrs), nil)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: fmt.Sprintf("[CQ:reply,id=%s]%s", history[0].MessageID, result), ShouldSave: true}, nil
	case index == reportIndex:
		dayNum, _ := strconv.Atoi(match[1])
		stats, err := s.store.ReportStats(ctx, groupID, startOfReport(dayNum), 10)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: formatReport(stats, dayNum, 10), ShouldSave: false}, nil
	case index == helpIndex:
		return &pendingOutbound{GroupID: groupID, Message: helpText, ShouldSave: false}, nil
	case index == helpBBHIndex:
		return &pendingOutbound{GroupID: groupID, Message: helpBBHText, ShouldSave: false}, nil
	case index == bbhIndex:
		result, err := s.handleBBHPlaza(ctx)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: false}, nil
	case index == bbhIndex+1:
		bookID, _ := strconv.Atoi(match[1])
		result, err := s.handleBBHBook(ctx, bookID)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: false}, nil
	case index == bbhIndex+2:
		bookID, _ := strconv.Atoi(match[1])
		para, _ := strconv.Atoi(match[2])
		result, err := s.handleBBHParagraphs(ctx, bookID, para, para)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: false}, nil
	case index == bbhIndex+3:
		bookID, _ := strconv.Atoi(match[1])
		left, _ := strconv.Atoi(match[2])
		right, _ := strconv.Atoi(match[3])
		result, err := s.handleBBHParagraphs(ctx, bookID, left, right)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: false}, nil
	case index == bbhIndex+4:
		bookID, _ := strconv.Atoi(match[1])
		author := match[2]
		content := match[3]
		result, err := s.handleBBHAddParagraph(ctx, bookID, author, content)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: false}, nil
	case index == bbhIndex+5:
		bookID, _ := strconv.Atoi(match[1])
		result, err := s.handleBBHAI(ctx, bookID)
		if err != nil {
			return nil, err
		}
		return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: false}, nil
	default:
		return nil, nil
	}
}

func (s *Service) handleNJKReply(ctx context.Context, event *napcat.GroupMessageEvent, groupID string) (*pendingOutbound, error) {
	history, err := s.historyStrings(ctx, groupID, randomRange(s.rng, 10, 30))
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, nil
	}

	var result string
	for i := 0; i < 5; i++ {
		temperature := 0.8 + s.rng.Float64()*0.1
		candidate, err := s.aiClient.Complete(ctx, prompts[njkIndex], fmt.Sprintf("%v", history), &temperature)
		if err != nil {
			return nil, err
		}
		if !containsExact(history, candidate) {
			result = candidate
			break
		}
		result = candidate
	}
	if result == "" {
		return nil, nil
	}
	return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: true}, nil
}

func (s *Service) historyStrings(ctx context.Context, groupID string, count int) ([]string, error) {
	messages, err := s.store.RecentMessages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	return formatStoredMessages(messages), nil
}

func formatStoredMessages(messages []StoredMessage) []string {
	result := make([]string, 0, len(messages))
	for _, item := range messages {
		result = append(result, item.Format())
	}
	return result
}

func (s *Service) saveIncomingMessageAndCheckImages(ctx context.Context, event *napcat.GroupMessageEvent) ([]DuplicateImage, error) {
	senderID := event.Sender.UserID.String()
	groupID := event.GroupID.String()
	if err := s.store.UpsertUser(ctx, senderID, event.Sender.Nickname); err != nil {
		return nil, err
	}
	groupName := event.GroupName
	if groupName == "" {
		groupName = groupID
	}
	if err := s.store.UpsertGroup(ctx, groupID, groupName); err != nil {
		return nil, err
	}

	textParts := []string{}
	atUsers := []string{}
	imageURLs := []string{}

	var replyID *string
	for _, segment := range event.Message.Segments {
		switch segment.Type {
		case "reply":
			id := segment.Data.ID.String()
			if id != "" {
				replyID = &id
			}
		case "at":
			userID := strings.TrimSpace(segment.Data.QQ)
			if userID == "" {
				continue
			}
			user, err := s.store.FindUser(ctx, userID)
			if err != nil {
				return nil, err
			}
			if user != nil {
				textParts = append(textParts, "@"+user.Nickname)
				atUsers = append(atUsers, user.UserID)
			} else {
				textParts = append(textParts, "@"+userID)
			}
		case "text":
			textParts = append(textParts, segment.Data.Text)
		case "image":
			if isEmojiImage(segment) {
				if err := s.imageService.EnsureEmojiWhitelist(ctx, groupID, segment.Data.URL); err != nil {
					log.Printf("【表情白名单处理失败】group=%s err=%v", groupID, err)
				}
				continue
			}
			if segment.Data.URL != "" {
				imageURLs = append(imageURLs, segment.Data.URL)
			}
		default:
			if segment.Data.Summary != "" {
				textParts = append(textParts, fmt.Sprintf("[%s: %s]", segment.Type, segment.Data.Summary))
			} else {
				textParts = append(textParts, "["+segment.Type+"]")
			}
		}
	}

	rawJSON, err := json.Marshal(event.Message.Segments)
	if err != nil {
		return nil, err
	}

	messageID := event.MessageID.String()
	messageText := strings.Join(textParts, "")
	senderIDCopy := senderID
	groupIDCopy := groupID
	card := emptyToNil(event.Sender.Card)
	text := emptyToNil(messageText)
	rawJSONString := string(rawJSON)
	rawMessage := emptyToNil(event.RawMessage)

	message := &model.Message{
		MessageID:  messageID,
		Time:       time.Unix(event.Time, 0),
		SenderID:   &senderIDCopy,
		GroupID:    &groupIDCopy,
		Card:       card,
		Text:       text,
		ReplyID:    replyID,
		RawJSON:    &rawJSONString,
		RawMessage: rawMessage,
	}
	if err := s.store.SaveMessage(ctx, message); err != nil {
		return nil, err
	}

	for _, userID := range atUsers {
		if err := s.store.SaveAtUser(ctx, messageID, userID); err != nil {
			return nil, err
		}
	}

	duplicates := []DuplicateImage{}
	for _, url := range imageURLs {
		duplicate, err := s.imageService.SaveAndCheckDuplicate(ctx, groupID, url, messageID)
		if err != nil {
			log.Printf("【图片消重失败】message=%s err=%v", messageID, err)
			continue
		}
		if duplicate != nil {
			duplicates = append(duplicates, *duplicate)
		}
	}

	return duplicates, nil
}

func (s *Service) saveSelfMessage(ctx context.Context, pending *pendingMessage, messageID string) error {
	if err := s.store.UpsertUser(ctx, s.cfg.BotUserID, s.cfg.BotNickname); err != nil {
		return err
	}
	if err := s.store.UpsertGroup(ctx, pending.GroupID, pending.GroupID); err != nil {
		return err
	}

	botUserID := s.cfg.BotUserID
	groupID := pending.GroupID
	text := pending.Message
	rawJSONBytes, err := json.Marshal(pending.Message)
	if err != nil {
		return err
	}
	rawJSON := string(rawJSONBytes)

	return s.store.SaveMessage(ctx, &model.Message{
		MessageID:  messageID,
		Time:       pending.SentAt,
		SenderID:   &botUserID,
		GroupID:    &groupID,
		Text:       &text,
		RawJSON:    &rawJSON,
		RawMessage: &text,
	})
}

func (s *Service) handleBBHPlaza(ctx context.Context) (string, error) {
	resp, err := s.bbhClient.Plaza(ctx)
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "获取失败", nil
	}
	allRead := []string{}
	allEdit := []string{}
	for _, book := range resp.Data {
		line := fmt.Sprintf("%d. %s", book.ID, book.Title)
		switch book.Scope {
		case "ALLREAD":
			allRead = append(allRead, line)
		case "ALLEDIT":
			allEdit = append(allEdit, line)
		}
	}
	return fmt.Sprintf("只读:\n%s\n\n可编辑:\n%s", strings.Join(allRead, "\n"), strings.Join(allEdit, "\n")), nil
}

func (s *Service) handleBBHBook(ctx context.Context, bookID int) (string, error) {
	bookResp, err := s.bbhClient.Book(ctx, bookID)
	if err != nil {
		return "", err
	}
	if !bookResp.Success {
		return fmt.Sprintf("获取%d号书失败", bookID), nil
	}

	parasResp, err := s.bbhClient.Paragraphs(ctx, bookID)
	if err != nil {
		return "", err
	}
	if !parasResp.Success {
		return "获取段落失败", nil
	}
	return fmt.Sprintf("%d. %s\n----------\n%s", bookResp.Data.ID, bookResp.Data.Title, paragraphsToTitles(trimBoundaryParagraphs(parasResp.Data))), nil
}

func (s *Service) handleBBHParagraphs(ctx context.Context, bookID int, left int, right int) (string, error) {
	resp, err := s.bbhClient.Paragraphs(ctx, bookID)
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "获取段落失败", nil
	}

	paras := trimBoundaryParagraphs(resp.Data)
	if left < 1 || right < 1 || left > len(paras) || right > len(paras) || left > right {
		return "段落索引错误", nil
	}

	lines := []string{}
	for i := left - 1; i < right; i++ {
		lines = append(lines, fmt.Sprintf("%d. %s\n%s", i+1, paras[i].Author, paras[i].Content))
	}
	return strings.Join(lines, "\n\n"), nil
}

func (s *Service) handleBBHAddParagraph(ctx context.Context, bookID int, author string, content string) (string, error) {
	resp, err := s.bbhClient.Paragraphs(ctx, bookID)
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "获取前段落失败", nil
	}
	paras := resp.Data
	if len(paras) < 2 {
		return "获取前段落失败", nil
	}

	added, err := s.bbhClient.AddParagraph(ctx, bbh.AddParagraphRequest{
		Author:     author,
		Content:    content,
		PrevParaID: paras[len(paras)-2].ID,
	})
	if err != nil {
		return "", err
	}
	if !added.Success {
		return "接龙失败", nil
	}
	return paragraphsToTitles(trimBoundaryParagraphs(paras)) + fmt.Sprintf("\n接龙成功: \n%d. %s", len(paras)-1, added.Data.Author), nil
}

func (s *Service) handleBBHAI(ctx context.Context, bookID int) (string, error) {
	resp, err := s.bbhClient.Paragraphs(ctx, bookID)
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "获取前段落失败", nil
	}
	paras := resp.Data
	if len(paras) < 2 {
		return "获取前段落失败", nil
	}

	type aiParagraph struct {
		Author  string `json:"author"`
		Content string `json:"content"`
	}
	paraContents := make([][2]string, 0, len(paras))
	for _, para := range paras {
		paraContents = append(paraContents, [2]string{para.Author, para.Content})
	}
	prompt := `你将会接收到一篇正在编写中的小说的每一个段落。其中author字段含义请自行视情况判断，有时候为作者，有时候为段标题，content字段则是段落正文内容。
现在请你理解前文，然后往下接一段。输出格式要求为json格式，一个字段"author"，一个字段"content"，字段值必须为字符串。接下来就是你将接收到的段落对象。`
	aiResult, err := s.aiClient.Complete(ctx, prompt, fmt.Sprintf("%v", paraContents), nil)
	if err != nil {
		return "", err
	}
	aiResult = strings.TrimSpace(aiResult)
	aiResult = strings.TrimPrefix(aiResult, "```json")
	aiResult = strings.TrimPrefix(aiResult, "```")
	aiResult = strings.TrimSuffix(aiResult, "```")
	aiResult = strings.TrimSpace(aiResult)

	var generated aiParagraph
	if err := json.Unmarshal([]byte(aiResult), &generated); err != nil {
		return "AI回答解析失败", nil
	}
	if generated.Author == "" || generated.Content == "" {
		return "AI回答格式错误", nil
	}

	added, err := s.bbhClient.AddParagraph(ctx, bbh.AddParagraphRequest{
		Author:     generated.Author,
		Content:    generated.Content,
		PrevParaID: paras[len(paras)-2].ID,
	})
	if err != nil {
		return "", err
	}
	if !added.Success {
		return "接龙失败", nil
	}
	return paragraphsToTitles(trimBoundaryParagraphs(paras)) + fmt.Sprintf("\n接龙成功: \n%d. %s", len(paras)-1, added.Data.Author), nil
}

func trimBoundaryParagraphs(paras []bbh.Paragraph) []bbh.Paragraph {
	if len(paras) <= 2 {
		return []bbh.Paragraph{}
	}
	return paras[1 : len(paras)-1]
}

func paragraphsToTitles(paras []bbh.Paragraph) string {
	lines := make([]string, 0, len(paras))
	for i, para := range paras {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, para.Author))
	}
	return strings.Join(lines, "\n")
}

func isEmojiImage(segment napcat.MessageSegment) bool {
	data := segment.Data
	return data.EmojiID != "" || data.EmojiPackageID != 0 || data.Key != "" || data.SubType == 1 || strings.Contains(data.Summary, "动画表情")
}

func mentionsBot(message napcat.MessagePayload, botUserID string) bool {
	for _, segment := range message.Segments {
		if segment.Type == "at" && strings.TrimSpace(segment.Data.QQ) == botUserID {
			return true
		}
	}
	return false
}

func emptyToNil(value string) *string {
	if value == "" {
		return nil
	}
	copyValue := value
	return &copyValue
}

func containsExact(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func randomRange(rng *rand.Rand, left int, right int) int {
	if right <= left {
		return left
	}
	return left + rng.Intn(right-left+1)
}

func startOfReport(dayNum int) time.Time {
	now := time.Now()
	todayFive := time.Date(now.Year(), now.Month(), now.Day(), 5, 0, 0, 0, now.Location())
	return todayFive.AddDate(0, 0, -dayNum)
}

func (s *Service) setLastAI(groupID string, at time.Time) {
	s.lastAIMu.Lock()
	defer s.lastAIMu.Unlock()
	s.lastAI[groupID] = at
}

func (s *Service) getLastAI(groupID string) (time.Time, bool) {
	s.lastAIMu.Lock()
	defer s.lastAIMu.Unlock()
	at, ok := s.lastAI[groupID]
	return at, ok
}

type pendingOutbound struct {
	GroupID    string
	Message    string
	ShouldSave bool
}

type pendingMessage struct {
	GroupID    string
	Message    string
	SentAt     time.Time
	ShouldSave bool
}

type pendingQueue struct {
	mu    sync.Mutex
	items []pendingMessage
}

func (q *pendingQueue) Push(item pendingMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *pendingQueue) Pop() *pendingMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	item := q.items[0]
	q.items = q.items[1:]
	return &item
}
