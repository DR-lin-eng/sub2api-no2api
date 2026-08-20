package egress

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"net/netip"
	"strings"
	"sync"
)

const maxAllocationAttempts = 64

type Allocator struct {
	mu     sync.RWMutex
	secret []byte
}

func NewAllocator(secret string) *Allocator {
	return &Allocator{secret: []byte(strings.TrimSpace(secret))}
}

func (a *Allocator) SetSecret(secret string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.secret = []byte(strings.TrimSpace(secret))
	a.mu.Unlock()
}

func (a *Allocator) SecretConfigured() bool {
	if a == nil {
		return false
	}
	a.mu.RLock()
	configured := len(a.secret) >= 32
	a.mu.RUnlock()
	return configured
}

func ValidatePoolCIDR(raw string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
	if err != nil || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
		return netip.Prefix{}, fmt.Errorf("invalid IPv6 prefix %q", raw)
	}
	prefix = prefix.Masked()
	if prefix.Bits() > 120 {
		return netip.Prefix{}, fmt.Errorf("IPv6 egress prefix %s is too small; /120 or larger is required", prefix)
	}
	return prefix, nil
}

// PoolCapacity returns the usable deterministic-address capacity. The all-zero
// host address is reserved so it cannot alias the prefix base address.
func PoolCapacity(raw string) (string, error) {
	prefix, err := ValidatePoolCIDR(raw)
	if err != nil {
		return "", err
	}
	capacity := new(big.Int).Lsh(big.NewInt(1), uint(128-prefix.Bits()))
	capacity.Sub(capacity, big.NewInt(1))
	return capacity.String(), nil
}

func (a *Allocator) Address(pool Pool, accountID, bindingVersion int64, attempt int) (string, error) {
	if a == nil {
		return "", ErrAllocationDisabled
	}
	a.mu.RLock()
	secret := append([]byte(nil), a.secret...)
	a.mu.RUnlock()
	if len(secret) == 0 {
		return "", ErrAllocationDisabled
	}
	if accountID <= 0 || pool.ID <= 0 || pool.AllocationVersion <= 0 || bindingVersion <= 0 || attempt < 0 {
		return "", fmt.Errorf("invalid IPv6 allocation inputs")
	}
	prefix, err := ValidatePoolCIDR(pool.CIDR)
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, secret)
	var input [40]byte
	binary.BigEndian.PutUint64(input[0:8], uint64(pool.ID))
	binary.BigEndian.PutUint64(input[8:16], uint64(pool.AllocationVersion))
	binary.BigEndian.PutUint64(input[16:24], uint64(accountID))
	binary.BigEndian.PutUint64(input[24:32], uint64(bindingVersion))
	binary.BigEndian.PutUint64(input[32:40], uint64(attempt))
	_, _ = mac.Write(input[:])
	digest := mac.Sum(nil)

	address := prefix.Addr().As16()
	for bit := prefix.Bits(); bit < 128; bit++ {
		hostBit := bit - prefix.Bits()
		value := (digest[hostBit/8] >> (7 - uint(hostBit%8))) & 1
		byteIndex := bit / 8
		mask := byte(1 << (7 - uint(bit%8)))
		if value == 1 {
			address[byteIndex] |= mask
		} else {
			address[byteIndex] &^= mask
		}
	}
	candidate := netip.AddrFrom16(address)
	if candidate == prefix.Addr() {
		address[15] |= 1
		candidate = netip.AddrFrom16(address)
	}
	if !prefix.Contains(candidate) {
		return "", fmt.Errorf("allocated address %s escaped prefix %s", candidate, prefix)
	}
	return candidate.String(), nil
}
