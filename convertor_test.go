package radix

import (
	"fmt"
	"strconv"
	"testing"
)

const maxElements = 70000000
const FullMsisdn = 79000000000

func BenchmarkName(b *testing.B) {
	tree := New()
	phone := "79017952592"
	phone = phone[2:]
	var i = uint32(0)
	for ; i <= maxElements; i++ {
		v := strconv.Itoa(int(i) + FullMsisdn)[2:]
		tree.Insert(v, nil)
	}

	_, ok := tree.Get(phone)
	v, _, ok := tree.Minimum()
	v, _, ok = tree.Maximum()

	if ok {
		fmt.Printf("нашли %s- длинна %d %v", "nil", tree.Len(), v)
	} else {

	}

	return
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, ok = tree.Get(phone)
	}
	b.StopTimer()
}
