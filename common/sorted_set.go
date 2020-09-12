package common

import (
	"sort"
)

// SortedSetInterface defines methods required by SortedSet helper API.
type SortedSetInterface interface {
	sort.Interface

	Pop(sz int)             // Pop removes `sz` elements from the tail.
	Push(x interface{})     // Push append `x` to the tail.
	Elem(i int) interface{} // Elem returns the element with index `i`.
}

// SortedSetEqualer defines Equal() method required by SortedSet helper API.
type SortedSetEqualer interface {
	Equal(i, j int) bool
}

func sortedSetDeduplicate(data SortedSetInterface) bool {
	eli, max, changed := 0, data.Len(), false

	fill := func(idx int) {
		if eli != idx {
			data.Swap(eli, idx)
			changed = true
		}
		eli++
	}

	equal, hasEqual := data.(SortedSetEqualer)

	for i := 1; i < max; i++ {
		equaled := false

		if hasEqual {
			equaled = equal.Equal(i-1, i)
		} else {
			ll, rl := data.Less(i-1, i), data.Less(i, i-1)
			equaled = ll == rl
		}

		if equaled {
			continue
		}
		fill(i - 1)
	}
	fill(max - 1)

	if eli < max {
		data.Pop(max - eli)
	}

	return changed
}

// SortedSetBuild builds sorted set.
func SortedSetBuild(data SortedSetInterface) {
	sort.Sort(data)
	sortedSetDeduplicate(data)
}

// SortedSetSubstract removes elements from left set.
func SortedSetSubstract(l, r SortedSetInterface, less func(x, y interface{}) bool) (changed bool) {
	if less == nil {
		panic("nil less comparator.")
	}

	changed = false
	if l.Len() > 0 && r.Len() > 0 {
		eli, lh := 0, 0
		for rh := 0; lh < l.Len() && rh < r.Len(); lh++ {
			le, re := l.Elem(lh), l.Elem(rh)
			lle, rle := less(le, re), less(re, le)
			if lle == rle {
				continue
			} else if rle {
				rh++
			}

			if eli != lh {
				l.Swap(eli, lh)
			}
		}
		if eli != lh {
			l.Pop(eli - lh)
			changed = true
		}
	}

	return
}

// SortedSetMerge merges two sorted set.
func SortedSetMerge(l, r SortedSetInterface) (changed bool) {
	changed = false

	// maps
	olh, rh := l.Len(), r.Len()
	for i := 0; i < rh; i++ {
		l.Push(r.Elem(i))
	}
	fMap, rMap := make([]int, olh+rh), make([]int, olh+rh)
	for i := 0; i < olh+rh; i++ {
		rMap[i], fMap[i] = i, i
	}
	rIdxMap, lIdxMap := fMap[olh:], fMap[:olh]

	// states
	widx, lh := l.Len(), olh
	left := func(lidx int) { // accept the left.
		if widx-1 != lidx {
			l.Swap(widx-1, lidx)
			lIdxMap[lh-1], fMap[rMap[widx-1]] = widx-1, lidx
			rMap[lidx], rMap[widx-1] = rMap[widx-1], lh-1
			changed = true
		}
		widx--
		lh--
	}
	right := func(ridx int) { // accept the right.
		if widx-1 != ridx {
			l.Swap(widx-1, ridx)
			rIdxMap[rh-1], fMap[rMap[widx-1]] = widx-1, ridx
			rMap[widx-1], rMap[ridx] = rh-1+olh, rMap[widx-1]
			changed = true
		}
		widx--
		rh--
	}

	// ops.
	for lh > 0 && rh > 0 {
		lidx, ridx := lIdxMap[lh-1], rIdxMap[rh-1]
		lle, rle := l.Less(lidx, ridx), l.Less(ridx, lidx)
		if lle || // left is less.
			lle == rle { // equal.
			left(lidx)
		} else {
			right(ridx)
		}
	}
	for lh > 0 {
		left(lIdxMap[lh-1])
	}
	for rh > 0 {
		right(rIdxMap[rh-1])
	}
	if sortedSetDeduplicate(l) {
		changed = true
	}

	return changed
}
