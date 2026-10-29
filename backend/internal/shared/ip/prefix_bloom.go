package ip

import (
	"math/bits"
	"net/netip"

	"github.com/cespare/xxhash/v2"
)

const (
	minBloomPrefixes = 64
	minBloomBits     = 64
	maxBloomBits     = 1 << 24
	bloomHashCount   = 3
)

// prefixBloom is a bounded, immutable Bloom filter for canonical network
// prefixes. It stores the prefix length with the address so a lookup can
// safely cover CIDR rules without enumerating their address space.
type prefixBloom struct {
	words   []uint64
	mask    uint64
	lengths [2][]uint8
}

func newPrefixBloom(prefixes []netip.Prefix) *prefixBloom {
	if len(prefixes) < minBloomPrefixes {
		return nil
	}

	var seen [2][129]bool
	lengthCount := 0
	for _, prefix := range prefixes {
		family := prefixFamily(prefix.Addr())
		if family == 0 {
			continue
		}
		length := prefix.Bits()
		if !seen[family-1][length] {
			seen[family-1][length] = true
			lengthCount++
		}
	}
	// Many distinct lengths require many probes. Prefer the linear path
	// unless each probe can save a meaningful number of prefix comparisons.
	if lengthCount == 0 || len(prefixes) < lengthCount*16 {
		return nil
	}

	// Ten bits per entry keeps the false-positive rate low while the cap
	// prevents a maliciously large API-key rule list from consuming unbounded
	// process memory. A power-of-two size makes the hot lookup modulo-free.
	requestedBits := uint64(len(prefixes)) * 10
	if requestedBits < minBloomBits {
		requestedBits = minBloomBits
	}
	if requestedBits > maxBloomBits {
		requestedBits = maxBloomBits
	}
	bitCount := uint64(minBloomBits)
	for bitCount < requestedBits {
		bitCount <<= 1
	}

	bloom := &prefixBloom{
		words: make([]uint64, bitCount/64),
		mask:  bitCount - 1,
	}
	for family := range seen {
		for length := range seen[family] {
			if seen[family][length] {
				bloom.lengths[family] = append(bloom.lengths[family], uint8(length))
			}
		}
	}
	for _, prefix := range prefixes {
		bloom.add(prefix)
	}
	return bloom
}

func prefixFamily(addr netip.Addr) int {
	if addr.Is4() {
		return 1
	}
	if addr.Is6() {
		return 2
	}
	return 0
}

func (b *prefixBloom) add(prefix netip.Prefix) {
	if b == nil {
		return
	}
	key := prefixBloomKey(prefix.Addr(), prefix.Bits())
	b.set(key)
}

func (b *prefixBloom) mayContain(addr netip.Addr) bool {
	if b == nil {
		return true
	}
	family := prefixFamily(addr)
	if family == 0 {
		return false
	}
	for _, length := range b.lengths[family-1] {
		if length == 0 {
			return true
		}
		if b.mayContainKey(prefixBloomKey(addr, int(length))) {
			return true
		}
	}
	return false
}

func (b *prefixBloom) set(key [18]byte) {
	h1, h2 := prefixBloomHashes(key)
	for i := uint64(0); i < bloomHashCount; i++ {
		index := (h1 + i*h2) & b.mask
		b.words[index>>6] |= uint64(1) << (index & 63)
	}
}

func (b *prefixBloom) mayContainKey(key [18]byte) bool {
	h1, h2 := prefixBloomHashes(key)
	for i := uint64(0); i < bloomHashCount; i++ {
		index := (h1 + i*h2) & b.mask
		if b.words[index>>6]&(uint64(1)<<(index&63)) == 0 {
			return false
		}
	}
	return true
}

func prefixBloomKey(addr netip.Addr, length int) [18]byte {
	var key [18]byte
	key[0] = byte(prefixFamily(addr))
	key[1] = byte(length)
	// Query the network at each configured length, rather than the host IP.
	address := netip.PrefixFrom(addr, length).Masked().Addr().As16()
	copy(key[2:], address[:])
	return key
}

func prefixBloomHashes(key [18]byte) (uint64, uint64) {
	h1 := xxhash.Sum64(key[:])
	// The odd step visits distinct bits in the power-of-two filter without
	// allocating a hash.Hash on each request.
	h2 := bits.RotateLeft64(h1^0x9e3779b97f4a7c15, 23) | 1
	return h1, h2
}
