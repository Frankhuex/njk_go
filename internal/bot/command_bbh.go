package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"njk_go/internal/bbh"
)

func (s *Service) handleBBHCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
	var (
		result string
		err    error
	)
	switch match.Command.Key {
	case commandBBHPlaza:
		result, err = s.handleBBHPlaza(ctx)
	case commandBBHBook:
		bookID, _ := strconv.Atoi(match.Groups[1])
		result, err = s.handleBBHBook(ctx, bookID)
	case commandBBHPara:
		bookID, _ := strconv.Atoi(match.Groups[1])
		para, _ := strconv.Atoi(match.Groups[2])
		result, err = s.handleBBHParagraphs(ctx, bookID, para, para)
	case commandBBHRange:
		bookID, _ := strconv.Atoi(match.Groups[1])
		left, _ := strconv.Atoi(match.Groups[2])
		right, _ := strconv.Atoi(match.Groups[3])
		result, err = s.handleBBHParagraphs(ctx, bookID, left, right)
	case commandBBHAdd:
		bookID, _ := strconv.Atoi(match.Groups[1])
		result, err = s.handleBBHAddParagraph(ctx, bookID, match.Groups[2], match.Groups[3])
	case commandBBHAI:
		bookID, _ := strconv.Atoi(match.Groups[1])
		result, err = s.handleBBHAI(ctx, bookID)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return simpleOutbound(groupID, result), nil
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
