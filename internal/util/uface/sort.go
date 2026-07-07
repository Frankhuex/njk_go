package uface

import (
	"sort"
	"strconv"
)

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

