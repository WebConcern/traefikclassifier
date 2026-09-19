package lib

import (
	"encoding/binary"
	"net"
	"sort"
)

// maxIPv4 is 255.255.255.255 as a uint32.
const maxIPv4 = ^uint32(0)

// ipv4Range is an inclusive range of IPv4 addresses.
type ipv4Range struct {
	start, end uint32
}

// ipSet answers "is this IP in any of these networks" in O(log n) for IPv4.
// IPv4 networks are merged into sorted, non-overlapping ranges for binary search.
// IPv6 networks are few in practice and are scanned linearly.
type ipSet struct {
	v4   []ipv4Range
	v6   []*net.IPNet
	size int // number of networks the set was built from
}

func newIPSet(nets []*net.IPNet) *ipSet {
	set := &ipSet{size: len(nets)}

	ranges := make([]ipv4Range, 0, len(nets))
	for _, n := range nets {
		ip4 := n.IP.To4()
		ones, bits := n.Mask.Size()
		if ip4 == nil || bits != 8*net.IPv4len {
			set.v6 = append(set.v6, n)
			continue
		}
		start := binary.BigEndian.Uint32(ip4)
		// Shifting a uint32 by 32 yields 0, so a /32 is a single address.
		hostMask := maxIPv4 >> ones
		ranges = append(ranges, ipv4Range{start: start, end: start | hostMask})
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	for _, r := range ranges {
		last := len(set.v4) - 1
		// Merge overlapping and adjacent ranges; guard the +1 against wrapping at 255.255.255.255.
		if last >= 0 && (r.start <= set.v4[last].end || (set.v4[last].end != maxIPv4 && r.start == set.v4[last].end+1)) {
			if r.end > set.v4[last].end {
				set.v4[last].end = r.end
			}
			continue
		}
		set.v4 = append(set.v4, r)
	}
	return set
}

func (s *ipSet) contains(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		n := binary.BigEndian.Uint32(ip4)
		// First range that does not end before n; ranges are sorted and disjoint.
		i := sort.Search(len(s.v4), func(i int) bool { return s.v4[i].end >= n })
		return i < len(s.v4) && s.v4[i].start <= n
	}
	for _, n := range s.v6 {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
