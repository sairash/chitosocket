package chitosocket

import (
	"crypto/rand"
	"fmt"
	"strings"
	"unsafe"
)

const (
	FNV_OFFSET_BASIS = 0x811c9dc5
	FNV_PRIME        = 0x01000193
	CHARS            = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	SERVER_ID_LEN    = 3 // we'll have 0 - 238327 servers denotated
	SESSION_LEN      = 32
)

var (
	LEN_CHARS = len(CHARS)
	MAX_CHARS = 256 - (256 % LEN_CHARS)
)

func fnv1Hash(input string) uint32 {
	cur := uint32(FNV_OFFSET_BASIS)
	for _, v := range input {
		cur ^= uint32(v)
		cur *= FNV_PRIME
	}
	return cur
}

func getShardFromString(input string, maxShards uint32) int {
	return int(fnv1Hash(input) % maxShards)
}

func randomKey(dest []byte) error {
	buf := [64]byte{}
	i := 0

	for i < len(dest) {
		if _, err := rand.Read(buf[:]); err != nil {
			return err
		}

		for _, v := range buf {
			if int(v) >= MAX_CHARS {
				continue
			}

			dest[i] = CHARS[int(v)%LEN_CHARS]
			i++

			if i == len(dest) {
				return nil
			}
		}
	}

	return nil
}

func encodeServerID(dest []byte, n int) error {
	if n < 0 {
		return fmt.Errorf("we can set server id as 0 or greater")
	}

	for i := 0; i < len(dest); i++ {
		dest[i] = CHARS[n%LEN_CHARS]
		n /= LEN_CHARS
	}

	if n != 0 {
		return fmt.Errorf("server ID too large")
	}
	return nil
}

func decodeServerID(p []byte) (int, error) {
	n := 0
	for i := 0; i < len(p); i++ {
		c := strings.IndexByte(CHARS, p[i])
		if c < 0 {
			return 0, fmt.Errorf("invalid server ID character %q", p[i])
		}
		n = n*LEN_CHARS + c
	}
	return n, nil
}

// n is the server id, keeping in mind that
// the cilent will be in multiple server
func randomSessionKey(n int) (string, error) {
	key := [SESSION_LEN]byte{}

	if err := encodeServerID(key[:SERVER_ID_LEN], n); err != nil {
		return "", err
	}

	if err := randomKey(key[SERVER_ID_LEN:]); err != nil {
		return "", err
	}

	return unsafe.String(unsafe.SliceData(key[:]), SESSION_LEN), nil
}

func unMaskBuffer(payload []byte, mask [4]byte) {
	for i := 0; i < len(payload); i++ {
		payload[i] ^= mask[i%4]
	}
}
