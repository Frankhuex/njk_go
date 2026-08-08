package uslice

import (
	"sort"
	"strconv"
)

func SortIntStrings(intStrings []string) {
	sort.SliceStable(intStrings, func(i, j int) bool {
		left, leftErr := strconv.ParseInt(intStrings[i], 10, 64)
		right, rightErr := strconv.ParseInt(intStrings[j], 10, 64)
		leftOK := leftErr == nil
		rightOK := rightErr == nil
		if leftOK && rightOK {
			if left == right {
				return intStrings[i] < intStrings[j]
			}
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return intStrings[i] < intStrings[j]
	})
}

// CompressIntRanges 把已按数值升序排序的整数字符串切片压缩成区间表示。
// 连续递增的整数合并为 "x-y"，单个整数保持原样，重复值会被去重。
// 例如 ["1","2","3","5","8","9"] → ["1-3","5","8-9"]。
// 非整数字符串保持原样单独输出。
func CompressIntRanges(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	start := items[0]
	prev := items[0]
	prevVal, prevErr := strconv.ParseInt(prev, 10, 64)
	for i := 1; i < len(items); i++ {
		cur := items[i]
		curVal, curErr := strconv.ParseInt(cur, 10, 64)
		if prevErr == nil && curErr == nil && curVal == prevVal {
			continue
		}
		if prevErr == nil && curErr == nil && curVal == prevVal+1 {
			prev = cur
			prevVal = curVal
			prevErr = curErr
			continue
		}
		result = append(result, joinRange(start, prev))
		start = cur
		prev = cur
		prevVal = curVal
		prevErr = curErr
	}
	result = append(result, joinRange(start, prev))
	return result
}

func joinRange(start, end string) string {
	if start == end {
		return start
	}
	return start + "-" + end
}
