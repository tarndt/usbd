package util

import (
	"fmt"
	"testing"
)

func TestIsZeros(t *testing.T) {
	cases := []struct {
		desc    string
		data    []byte
		isZeros bool
	}{
		{
			desc:    "nil",
			data:    nil,
			isZeros: true,
		},
		{
			desc:    "empty",
			data:    []byte{},
			isZeros: true,
		},
		{
			desc:    "single-zero",
			data:    []byte{0},
			isZeros: true,
		},
		{
			desc:    "two-zero",
			data:    make([]byte, 2),
			isZeros: true,
		},
		{
			desc:    "seven-zero",
			data:    make([]byte, 7),
			isZeros: true,
		},
		{
			desc:    "eight-zero",
			data:    make([]byte, 8),
			isZeros: true,
		},
		{
			desc:    "nine-zero",
			data:    make([]byte, 9),
			isZeros: true,
		},
		{
			desc:    "huge-zero-aligned",
			data:    make([]byte, 256),
			isZeros: true,
		},
		{
			desc:    "huge-zero-unaligned",
			data:    make([]byte, 257),
			isZeros: true,
		},
		{
			desc:    "two-one",
			data:    oneFill(make([]byte, 2)),
			isZeros: false,
		},
		{
			desc:    "seven-one",
			data:    oneFill(make([]byte, 7)),
			isZeros: false,
		},
		{
			desc:    "eight-one",
			data:    oneFill(make([]byte, 8)),
			isZeros: false,
		},
		{
			desc:    "nine-one",
			data:    oneFill(make([]byte, 9)),
			isZeros: false,
		},
		{
			desc:    "huge-one-aligned",
			data:    oneFill(make([]byte, 256)),
			isZeros: false,
		},
		{
			desc:    "huge-one-unaligned",
			data:    oneFill(make([]byte, 257)),
			isZeros: false,
		},
		{
			desc:    "zero-one",
			data:    []byte{0, 1},
			isZeros: false,
		},
		{
			desc:    "eight-zero-single-one",
			data:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 1},
			isZeros: false,
		},
		{
			desc:    "nine-zero-single-one",
			data:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			isZeros: false,
		},
	}

	for _, tcase := range cases {
		t.Run(tcase.desc, func(t *testing.T) {
			if actual := IsZeros(tcase.data); actual != tcase.isZeros {
				t.Fatalf("isZero(%v) returned %t rather than %t", tcase.data, actual, tcase.isZeros)
			}
		})
	}
}

func oneFill(data []byte) []byte {
	for i := range data {
		data[i] = 1
	}
	return data
}

func TestZeroFill(t *testing.T) {
	t.Run("count-nil", func(t *testing.T) {
		if ZeroFill(nil); !IsZeros(nil) {
			t.Fatal("Empty block was not correctly zero-filled")
		}
	})

	for count := 1; count <= 8192*4; count <<= 1 {
		t.Run(fmt.Sprintf("count-%d", count), func(t *testing.T) {
			block := oneFill(make([]byte, count))
			if ZeroFill(block); !IsZeros(block) {
				t.Fatalf("Block of %d ones was not correctly zero-filled", count)
			}
		})
	}
}

func BenchmarkIsZeros(b *testing.B) {
	block := make([]byte, b.N)
	b.ReportAllocs()
	b.ResetTimer()

	IsZeros(block)
}

func BenchmarkZerofill(b *testing.B) {
	block := make([]byte, b.N)
	b.ReportAllocs()
	b.ResetTimer()

	ZeroFill(block)
}
