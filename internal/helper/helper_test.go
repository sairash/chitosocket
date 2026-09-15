package helper

import (
	"testing"
)

const MAX_SHARDS = 8

var fnvTests = []struct {
	input         string
	expetedFNV    uint32
	expectedShard int
}{
	{"hello", 1335831723, 3},
	{"test", 2949673445, 5},
	{"Hello World", 3012568359, 7},
	{"example.com", 1125968678, 6},
	{"user@email.com", 3116055849, 1},
}

func TestFnv1Hash(t *testing.T) {
	for _, tt := range fnvTests {
		ret := fnv1Hash(tt.input)
		if ret != tt.expetedFNV {
			t.Errorf("for input %s, expected %d, got %d", tt.input, tt.expetedFNV, ret)
		}
	}
}

func TestShardNumber(t *testing.T) {
	for _, tt := range fnvTests {
		ret := GetShardFromString(tt.input, MAX_SHARDS)
		if ret != tt.expectedShard {
			t.Errorf("for input %s, expected %d, got %d", tt.input, tt.expectedShard, ret)
		}
	}
}
