package pagination

import (
	"math"
	"testing"
)

func TestOffsetSaturatesWithoutOverflow(t *testing.T) {
	for _, size := range []int{-1, 1, 20, 1000, math.MaxInt} {
		for _, page := range []int{-1, 0, 1, 2, math.MaxInt / 2, math.MaxInt} {
			p := PaginationParams{Page: page, PageSize: size}
			offset := p.Offset()
			if offset < 0 || offset > math.MaxInt-p.Limit() {
				t.Fatalf("page=%d size=%d offset=%d limit=%d", page, size, offset, p.Limit())
			}
			if page == 2 && offset != p.Limit() {
				t.Fatalf("ordinary second page changed: %+v", p)
			}
		}
	}
}
