package runner

import "testing"

func TestHeapMB(t *testing.T) {
	cases := []struct {
		memoryMB int
		want     int
	}{
		{256, 128},   // ขั้นต่ำที่ API ยอม — reserve โดน cap ที่ครึ่งหนึ่ง
		{512, 256},   // reserve floor 256
		{1024, 683},  //
		{2048, 1366}, //
		{3072, 2048}, //
		{8192, 6144}, // reserve โดน cap ที่ 2048
		{16384, 14336},
	}
	for _, c := range cases {
		if got := HeapMB(c.memoryMB); got != c.want {
			t.Errorf("HeapMB(%d) = %d, want %d", c.memoryMB, got, c.want)
		}
	}
}

// heap ต้องน้อยกว่า limit เสมอและมากกว่า 0 ไม่ว่าค่าไหน — ถ้าหลุดคือ container start ไม่ขึ้น
// หรือโดน OOM kill ทันที
func TestHeapMBAlwaysLeavesRoom(t *testing.T) {
	for mem := 256; mem <= 65536; mem += 37 {
		heap := HeapMB(mem)
		if heap <= 0 {
			t.Fatalf("HeapMB(%d) = %d, ต้องมากกว่า 0", mem, heap)
		}
		if heap >= mem {
			t.Fatalf("HeapMB(%d) = %d, ต้องน้อยกว่า limit", mem, heap)
		}
	}
}
