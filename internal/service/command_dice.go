package service

import (
	"context"
	"strconv"
	"strings"
)

func (s *Service) handleDiceCommand(_ context.Context, groupID string, match CommandMatch) (*pendingOutbound, error) {
	if len(match.Groups) < 3 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	count, err := strconv.Atoi(match.Groups[1])
	if err != nil {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	sides, err := strconv.Atoi(match.Groups[2])
	if err != nil {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	if count < 1 || sides < 1 {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	if count > 20 {
		return simpleOutbound(groupID, "太多啦，最多20次"), nil
	}

	results := make([]int, 0, count)
	resultStrs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		results = append(results, randomRange(s.rng, 1, sides))
		resultStrs = append(resultStrs, strconv.Itoa(results[i]))
	}
	return simpleOutbound(groupID, strings.Join(resultStrs, "+")+"="+strconv.Itoa(sum(results))), nil
}

func sum(results []int) int {
	sum := 0
	for _, v := range results {
		sum += v
	}
	return sum
}
