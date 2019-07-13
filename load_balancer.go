package gproxy

type node struct {
	cw int
	ew int
	id int
}

// swrr
// @TODO 参考cpu/mem
func swrr(s []*node) *node {
	var best *node
	tw := 0
	for _, n := range s {
		n.cw += n.ew
		tw += n.ew
		if best == nil || n.cw > best.cw {
			best = n
		}
	}
	best.cw -= tw
	return best
}
