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
