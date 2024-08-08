package radix

import (
	"strconv"
	"testing"
)

const maxElements = 70000000
const FullMsisdn = 79000000000

// BenchmarkName-8   	10960039	       108.4 ns/op	       0 B/op	       0 allocs/op
func BenchmarkName(b *testing.B) {
	tree := New()
	phone := "79017952599"
	phone = phone[2:]
	var i = uint32(0)
	for ; i <= maxElements; i++ {
		v := strconv.Itoa(int(i) + FullMsisdn)[2:]
		tree.Insert(v, nil)
	}

	tree.Optimize()

	b.ResetTimer()
	b.ReportAllocs()

	var ok bool
	for i := 0; i < b.N; i++ {
		_, ok = tree.Get(phone)
	}
	b.StopTimer()
	if ok {

	}
}
