package service

const (
	modelMappingSignatureFNVOffset = 14695981039346656037
	modelMappingSignatureFNVPrime  = 1099511628211
)

// modelMappingSignature 计算原始映射内容的进程内校验值，与 map 遍历顺序无关且不分配内存。
// 每个条目把 key、分隔字节 0 与 value（string 写其字节，其它类型写标记字节 1）
// 放进同一个 FNV-1a 哈希并经 fmix64 收尾，再对所有条目求和；
// key 与 value 同在一个条目哈希里，交换两个 key 的 value 会改变结果。
func modelMappingSignature(rawMapping map[string]any) uint64 {
	if len(rawMapping) == 0 {
		return 0
	}
	var sum uint64
	for k, v := range rawMapping {
		h := uint64(modelMappingSignatureFNVOffset)
		h = fnv1aString(h, k)
		h = fnv1aByte(h, 0)
		if s, ok := v.(string); ok {
			h = fnv1aString(h, s)
		} else {
			h = fnv1aByte(h, 1)
		}
		sum += fmix64(h)
	}
	return sum
}

func fnv1aString(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= modelMappingSignatureFNVPrime
	}
	return h
}

func fnv1aByte(h uint64, b byte) uint64 {
	h ^= uint64(b)
	h *= modelMappingSignatureFNVPrime
	return h
}

// fmix64 是 MurmurHash3 的 64 位收尾函数，使条目哈希各位充分混合后再求和。
func fmix64(h uint64) uint64 {
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return h
}
