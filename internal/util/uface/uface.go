package uface

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"njk_go/internal/napcat"
)

func FaceIDsFromSegments(segments []napcat.MessageSegment) []string {
	seen := make(map[string]struct{}, len(segments))
	faceIDs := make([]string, 0)
	for _, segment := range segments {
		if segment.Type != napcat.SegmentTypeFace {
			continue
		}
		faceID := strings.TrimSpace(segment.Data.ID.String())
		if faceID == "" {
			continue
		}
		if _, ok := seen[faceID]; ok {
			continue
		}
		seen[faceID] = struct{}{}
		faceIDs = append(faceIDs, faceID)
	}
	return faceIDs
}

func EmojiLikeFaceIDs(likes []napcat.EmojiLike) []string {
	seen := map[string]struct{}{}
	faceIDs := make([]string, 0, len(likes))
	add := func(faceID string) {
		faceID = strings.TrimSpace(faceID)
		if faceID == "" {
			return
		}
		if _, ok := seen[faceID]; ok {
			return
		}
		seen[faceID] = struct{}{}
		faceIDs = append(faceIDs, faceID)
	}
	for _, like := range likes {
		add(like.EmojiID)
	}
	return faceIDs
}

func ExtractFaceIDsFromRawJSON(rawJSON string) ([]string, error) {
	if rawJSON == "" {
		return nil, nil
	}
	var segments []napcat.MessageSegment
	if err := json.Unmarshal([]byte(rawJSON), &segments); err != nil {
		return nil, err
	}
	return FaceIDsFromSegments(segments), nil
}

func SortFaceIDs(faceIDs []string) {
	sort.SliceStable(faceIDs, func(i, j int) bool {
		left, leftErr := strconv.ParseInt(faceIDs[i], 10, 64)
		right, rightErr := strconv.ParseInt(faceIDs[j], 10, 64)
		leftOK := leftErr == nil
		rightOK := rightErr == nil
		if leftOK && rightOK {
			if left == right {
				return faceIDs[i] < faceIDs[j]
			}
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return faceIDs[i] < faceIDs[j]
	})
}
