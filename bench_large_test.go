package structwatcher

import (
	"testing"
	"time"
)

// 本文件量化大结构体（50+ 字段）与深嵌套结构体的最坏情况开销。
// 运行：go test -bench 'Large|Deep' -benchmem -run '^$' .

// ===== 扁平大结构体：55 个被追踪字段（混合标量与引用类型） =====

type LargeFlat struct {
	*Watcher[LargeFlat]
	Str0, Str1, Str2, Str3, Str4, Str5, Str6, Str7, Str8, Str9, Str10, Str11 string
	Int0, Int1, Int2, Int3, Int4, Int5, Int6, Int7, Int8, Int9, Int10, Int11 int
	Big0, Big1, Big2, Big3, Big4, Big5, Big6, Big7                           int64
	Flt0, Flt1, Flt2, Flt3, Flt4, Flt5, Flt6, Flt7                           float64
	Flag0, Flag1, Flag2, Flag3, Flag4, Flag5                                 bool
	At, UpdatedAt                                                            time.Time
	List0, List1, List2, List3                                               []string
	Counters, Weights                                                        map[string]int
	Priority                                                                 *int
}

func initLarge() LargeFlat {
	n := 42
	list := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	m := make(map[string]int, 10)
	for i := 0; i < 10; i++ {
		m[string(rune('a'+i))] = i
	}
	return LargeFlat{
		Str0: "alpha", Str1: "bravo", Str2: "charlie", Str3: "delta",
		Int0: 1, Int1: 2, Int2: 3, Int3: 4,
		Big0: 1 << 40, Flt0: 3.14, Flt1: 2.71, Flag0: true, Flag1: true,
		At:        time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 26, 13, 0, 0, 0, time.UTC),
		List0:     list, List1: list, List2: list, List3: list,
		Counters: m, Weights: m,
		Priority: &n,
	}
}

// ===== 深嵌套结构体：5 层嵌套，约 50 个叶子字段 =====

type Leaf struct {
	A string
	B int
	C []string
}

type Mid1 struct {
	L1, L2 Leaf
	S      string
}

type Mid2 struct {
	M1, M2 Mid1
	N      int
}

type Level3 struct {
	X1, X2 Mid2
	T      time.Time
}

type DeepNested struct {
	*Watcher[DeepNested]
	Root  Level3
	Extra map[string]int
}

func initDeep() DeepNested {
	leaf := Leaf{A: "leaf", B: 1, C: []string{"x", "y", "z"}}
	mid1 := Mid1{L1: leaf, L2: leaf, S: "mid"}
	mid2 := Mid2{M1: mid1, M2: mid1, N: 2}
	l3 := Level3{X1: mid2, X2: mid2, T: time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)}
	return DeepNested{
		Root:  l3,
		Extra: map[string]int{"k1": 1, "k2": 2, "k3": 3},
	}
}

// ===== 扁平大结构体基准 =====

func BenchmarkLargeFlat_New(b *testing.B) {
	v := initLarge()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = New(v)
	}
}

func BenchmarkLargeFlat_IsChanged_NoChange(b *testing.B) {
	p := New(initLarge())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p.IsChanged() {
			b.Fatal("unexpected change")
		}
	}
}

// 修改声明顺序中最后一个字段：遍历成本最坏情况
func BenchmarkLargeFlat_IsChanged_LastFieldChanged(b *testing.B) {
	p := New(initLarge())
	*p.Priority = 43
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !p.IsChanged() {
			b.Fatal("expected change")
		}
	}
}

func BenchmarkLargeFlat_Changes_OneChange(b *testing.B) {
	p := New(initLarge())
	p.Str0 = "changed"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Changes()
	}
}

func BenchmarkLargeFlat_Changes_TenChanges(b *testing.B) {
	p := New(initLarge())
	p.Str0, p.Str1, p.Str2 = "c1", "c2", "c3"
	p.Int0, p.Int1, p.Int2 = 10, 11, 12
	p.Flag0, p.Flag1 = false, false
	p.Flt0, p.Flt1 = 1.1, 2.2
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Changes()
	}
}

func BenchmarkLargeFlat_Reset(b *testing.B) {
	p := New(initLarge())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Reset()
	}
}

// 重负载 Reset：量化深拷贝随数据体积的扩展成本
// （4 个 1 万元素 slice + 2 个 1 千条目 map）
func BenchmarkLargeFlat_Reset_Heavy(b *testing.B) {
	v := initLarge()
	big := make([]string, 10_000)
	for i := range big {
		big[i] = "elem"
	}
	bigMap := make(map[string]int, 1_000)
	for i := 0; i < 1_000; i++ {
		bigMap[string(rune('a'+i%26))+string(rune('a'+i/26))] = i
	}
	v.List0, v.List1, v.List2, v.List3 = big, big, big, big
	v.Counters, v.Weights = bigMap, bigMap
	p := New(v)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Reset()
	}
}

// ===== 深嵌套结构体基准 =====

func BenchmarkDeepNested_New(b *testing.B) {
	v := initDeep()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = New(v)
	}
}

func BenchmarkDeepNested_IsChanged_NoChange(b *testing.B) {
	d := New(initDeep())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if d.IsChanged() {
			b.Fatal("unexpected change")
		}
	}
}

// 修改最深层的叶子字段（第 6 层），并在 Reset 后原地修改 slice：
// 同时压测 DeepEqual 递归与快照深拷贝
func BenchmarkDeepNested_IsChanged_DeepLeafInPlace(b *testing.B) {
	d := New(initDeep())
	d.Reset()
	d.Root.X2.M2.L2.C[0] = "mutated"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !d.IsChanged() {
			b.Fatal("expected change")
		}
	}
}

func BenchmarkDeepNested_Reset(b *testing.B) {
	d := New(initDeep())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Reset()
	}
}

// ===== 扩展性归因基准：各类字段对成本的贡献 =====
// 同一 55 字段结构体，逐类启用字段，定位开销来源：
// 标量/切片/时间字段均为零分配；map 比较约 3 allocs/条目（reflect 固有）。

type LS struct {
	*Watcher[LS]
	S0, S1, S2, S3, S4, S5, S6, S7, S8, S9, S10, S11 string
	I0, I1, I2, I3, I4, I5, I6, I7, I8, I9, I10, I11 int
	B0, B1, B2, B3, B4, B5, B6, B7                   int64
	F0, F1, F2, F3, F4, F5, F6, F7                   float64
	G0, G1, G2, G3, G4, G5                           bool
	At, UpdatedAt                                    time.Time
	L0, L1, L2, L3                                   []string
	M0, M1                                           map[string]int
}

func lsBase() LS {
	return LS{S0: "a", S1: "b", I0: 1, I1: 2, B0: 1 << 40, F0: 3.14, G0: true}
}

func lsTime() LS {
	v := lsBase()
	v.At = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	v.UpdatedAt = time.Date(2026, 8, 26, 13, 0, 0, 0, time.UTC)
	return v
}

func lsSlices() LS {
	v := lsTime()
	l := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	v.L0, v.L1, v.L2, v.L3 = l, l, l, l
	return v
}

func lsFull() LS {
	v := lsSlices()
	m := make(map[string]int, 10)
	for i := 0; i < 10; i++ {
		m[string(rune('a'+i))] = i
	}
	v.M0, v.M1 = m, m
	return v
}

func BenchmarkScale_Scalars(b *testing.B) {
	p := New(lsBase())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p.IsChanged() {
			b.Fatal("unexpected change")
		}
	}
}

func BenchmarkScale_PlusTime(b *testing.B) {
	p := New(lsTime())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p.IsChanged() {
			b.Fatal("unexpected change")
		}
	}
}

func BenchmarkScale_PlusSlices(b *testing.B) {
	p := New(lsSlices())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p.IsChanged() {
			b.Fatal("unexpected change")
		}
	}
}

func BenchmarkScale_PlusMaps(b *testing.B) {
	p := New(lsFull())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p.IsChanged() {
			b.Fatal("unexpected change")
		}
	}
}
