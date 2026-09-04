package search

import (
	"strings"
	"unicode"
)

const (
	bonusConsecutive    = 8
	bonusWordBoundary   = 10
	bonusCamelCase      = 8
	bonusDotSeparator   = 9
	bonusFirstChar      = 12
	bonusCaseMatch      = 1
	penaltyGap          = -3
	penaltyLeadingGap   = -1
	penaltyUnmatched    = -100
)

type FuzzyMatch struct {
	Score     int
	Positions []int
}

func FuzzyScore(query, target string) FuzzyMatch {
	if len(query) == 0 {
		return FuzzyMatch{Score: 0}
	}
	if len(target) == 0 {
		return FuzzyMatch{Score: penaltyUnmatched}
	}

	queryLower := strings.ToLower(query)
	targetLower := strings.ToLower(target)

	qi := 0
	for ti := 0; ti < len(targetLower); ti++ {
		if qi < len(queryLower) && targetLower[ti] == queryLower[qi] {
			qi++
		}
	}
	if qi != len(queryLower) {
		return FuzzyMatch{Score: penaltyUnmatched}
	}

	n := len(target)
	m := len(query)

	score := make([][]int, m)
	consecutive := make([][]int, m)
	for i := range score {
		score[i] = make([]int, n)
		consecutive[i] = make([]int, n)
		for j := range score[i] {
			score[i][j] = -1 << 30
		}
	}

	for j := 0; j < n; j++ {
		if targetLower[j] != queryLower[0] {
			continue
		}

		s := 0
		if j == 0 {
			s += bonusFirstChar
		}
		if query[0] == target[j] {
			s += bonusCaseMatch
		}
		if isWordBoundary(target, j) {
			s += bonusWordBoundary
		}
		if j > 0 && target[j-1] == '.' {
			s += bonusDotSeparator
		}

		s += j * penaltyLeadingGap

		score[0][j] = s
		consecutive[0][j] = 1
	}

	for i := 1; i < m; i++ {
		for j := i; j < n; j++ {
			if targetLower[j] != queryLower[i] {
				continue
			}

			bestScore := -1 << 30
			bestConsec := 0

			for k := i - 1; k < j; k++ {
				if score[i-1][k] == -1<<30 {
					continue
				}

				s := score[i-1][k]
				gap := j - k - 1

				if gap == 0 {
					c := consecutive[i-1][k] + 1
					s += bonusConsecutive * c
					if c > bestConsec {
						bestConsec = c
					}
				} else {
					s += gap * penaltyGap
					bestConsec = 1
				}

				if query[i] == target[j] {
					s += bonusCaseMatch
				}
				if isWordBoundary(target, j) {
					s += bonusWordBoundary
				}
				if isCamelBoundary(target, j) {
					s += bonusCamelCase
				}
				if j > 0 && target[j-1] == '.' {
					s += bonusDotSeparator
				}

				if s > bestScore {
					bestScore = s
				}
			}

			score[i][j] = bestScore
			consecutive[i][j] = bestConsec
		}
	}

	bestScore := -1 << 30
	bestEnd := -1
	for j := m - 1; j < n; j++ {
		if score[m-1][j] > bestScore {
			bestScore = score[m-1][j]
			bestEnd = j
		}
	}

	if bestScore <= penaltyUnmatched {
		return FuzzyMatch{Score: penaltyUnmatched}
	}

	positions := make([]int, m)
	positions[m-1] = bestEnd
	for i := m - 2; i >= 0; i-- {
		best := -1
		bestS := -1 << 30
		for k := i; k < positions[i+1]; k++ {
			if score[i][k] > bestS {
				bestS = score[i][k]
				best = k
			}
		}
		positions[i] = best
	}

	return FuzzyMatch{
		Score:     bestScore,
		Positions: positions,
	}
}

func isWordBoundary(s string, i int) bool {
	if i == 0 {
		return true
	}
	prev := s[i-1]
	return prev == '/' || prev == '\\' || prev == '_' || prev == '-' || prev == '.' || prev == ' '
}

func isCamelBoundary(s string, i int) bool {
	if i == 0 || i >= len(s) {
		return false
	}
	return unicode.IsUpper(rune(s[i])) && unicode.IsLower(rune(s[i-1]))
}
