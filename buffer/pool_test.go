package buffer

import "testing"

func TestLog2(t *testing.T) {
	logTest := []struct {
		input    int
		expected int
	}{
		{1, 1},
		{2, 2},

		{7, 3},

		{8, 4},
		{15, 4},

		{1023, 10},
		{1024, 11},

		{4096, 13},

		{2147483647, 31},
	}
	for _, v := range logTest {
		r := exponent(v.input)
		if v.expected != r {
			t.Errorf("in log2 for value %d, expected: %d, got: %d", v.input, v.expected, r)
		}
	}
}

func TestAntiLog(t *testing.T) {
	antiLogTest := []struct {
		input    int
		expected int
	}{
		{0, 1},
		{1, 2},
		{2, 4},

		{3, 8},
		{4, 16},

		{10, 1024},

		{15, 32768},

		{31, 2147483648},
	}
	for _, v := range antiLogTest {
		r := antiLog(v.input)
		if v.expected != r {
			t.Errorf("in antilog for value %d, expected: %d, got: %d", v.input, v.expected, r)
		}
	}
}

func TestGetAndPut(t *testing.T) {
	bufferPool := new(pool)

	getPutTest := []struct {
		input int
	}{
		{1024},
		{8848},
		{1000},
	}

	for _, v := range getPutTest {
		bufferPool.Put(make([]byte, v.input))
	}

	for _, v := range getPutTest {
		b := bufferPool.Get(v.input)
		lenB := len(b)

		if v.input != lenB {
			t.Errorf("expected byte len as %d got %d", v.input, lenB)
		}
	}
}
