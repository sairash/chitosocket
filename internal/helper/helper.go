package helper

const (
	FNV_OFFSET_BASIS = 0x811c9dc5
	FNV_PRIME        = 0x01000193
)

func fnv1Hash(input string) uint32 {
	cur := uint32(FNV_OFFSET_BASIS)
	for _, v := range input {
		cur ^= uint32(v)
		cur *= FNV_PRIME
	}
	return cur
}

func GetShardFromString(input string, maxShards uint32) int {
	return int(fnv1Hash(input) % maxShards)
}
