package structwatcher

import (
	"math"
	"reflect"
	"testing"
	"time"
)

type Person struct {
	*Watcher[Person]
	Name string
	Age  int
}

type Config struct {
	*Watcher[Config]
	Title   string
	Enabled bool
	Tags    []string
	M       map[string]int
	Count   int
	private string
}

type Box struct {
	*Watcher[Box]
	N *int
}

type Callbacks struct {
	*Watcher[Callbacks]
	OnEvent func() error
}

type Channel struct {
	*Watcher[Channel]
	C chan int
}

type Metrics struct {
	*Watcher[Metrics]
	Score float64
}

type Event struct {
	*Watcher[Event]
	At time.Time
}

type fakeWatchable struct {
	Name string
}

func (fakeWatchable) IsChanged() bool   { return false }
func (fakeWatchable) Changes() []Change { return nil }

type Document struct {
	*Watcher[Document]
	Title     string `watch:"-"`
	Version   int    `watch:"-"`
	Body      string
	History   []string `watch:"-"`
	Reviewers []string
	private   string
}

type BadTag struct {
	*Watcher[BadTag]
	Field string `watch:"ignore"`
}

func TestNew_Success(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	if p == nil {
		t.Fatal("New returned nil")
	}
	if p.Name != "tom" {
		t.Errorf("expected Name=tom, got %q", p.Name)
	}
	if p.Age != 20 {
		t.Errorf("expected Age=20, got %d", p.Age)
	}
	if p.Watcher == nil {
		t.Fatal("embedded Watcher is nil")
	}
}

func TestNew_InitialNoChanges(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	if p.IsChanged() {
		t.Error("newly created struct should have no changes")
	}

	changes := p.Changes()
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %d", len(changes))
	}
}

func TestIsChanged_NoChange(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	if got := p.IsChanged(); got {
		t.Errorf("expected false, got true")
	}
}

func TestIsChanged_SingleField(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	p.Name = "jerry"

	if got := p.IsChanged(); !got {
		t.Errorf("expected true, got false")
	}
}

func TestIsChanged_MultipleFields(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	p.Name = "jerry"
	p.Age = 25

	if got := p.IsChanged(); !got {
		t.Errorf("expected true, got false")
	}
}

func TestIsChanged_NilWatcher(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil watcher")
		}
	}()

	var w *Watcher[Person]
	w.IsChanged()
}

func TestChanges_NoChange(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	changes := p.Changes()
	if changes != nil && len(changes) != 0 {
		t.Errorf("expected nil or empty changes, got %v", changes)
	}
}

func TestChanges_SingleField(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	oldName := p.Name
	p.Name = "jerry"

	changes := p.Changes()

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	if changes[0].Field != "Name" {
		t.Errorf("expected Field=Name, got %q", changes[0].Field)
	}
	if changes[0].OldValue != oldName {
		t.Errorf("expected OldValue=%q, got %v", oldName, changes[0].OldValue)
	}
	if changes[0].NewValue != "jerry" {
		t.Errorf("expected NewValue=jerry, got %v", changes[0].NewValue)
	}
}

func TestChanges_MultipleFields(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	p.Name = "jerry"
	p.Age = 25

	changes := p.Changes()

	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}

	changeSet := make(map[string]Change)
	for _, c := range changes {
		changeSet[c.Field] = c
	}

	nameChange, ok := changeSet["Name"]
	if !ok {
		t.Fatal("missing Name change")
	}
	if nameChange.OldValue != "tom" || nameChange.NewValue != "jerry" {
		t.Errorf("Name change: expected tom->jerry, got %v->%v", nameChange.OldValue, nameChange.NewValue)
	}

	ageChange, ok := changeSet["Age"]
	if !ok {
		t.Fatal("missing Age change")
	}
	if ageChange.OldValue != 20 || ageChange.NewValue != 25 {
		t.Errorf("Age change: expected 20->25, got %v->%v", ageChange.OldValue, ageChange.NewValue)
	}
}

func TestChanges_NilWatcher(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil watcher")
		}
	}()

	var w *Watcher[Person]
	w.Changes()
}

func TestChanges_UnexportedFieldsSkipped(t *testing.T) {
	c := New(Config{
		Title:   "test",
		Enabled: false,
		Tags:    []string{"a"},
		Count:   1,
		private: "secret",
	})

	c.Title = "modified"
	c.private = "new-secret"

	changes := c.Changes()

	if len(changes) != 1 {
		t.Fatalf("expected 1 change (only exported fields), got %d", len(changes))
	}

	if changes[0].Field != "Title" {
		t.Errorf("expected Field=Title, got %q", changes[0].Field)
	}
}

func TestChanges_SliceField(t *testing.T) {
	c := New(Config{
		Title: "test",
		Tags:  []string{"a", "b"},
	})

	c.Tags = []string{"a", "b", "c"}

	changes := c.Changes()

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	if changes[0].Field != "Tags" {
		t.Errorf("expected Field=Tags, got %q", changes[0].Field)
	}
}

func TestChanges_BoolField(t *testing.T) {
	c := New(Config{
		Title:   "test",
		Enabled: false,
	})

	c.Enabled = true

	changes := c.Changes()

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	if changes[0].Field != "Enabled" {
		t.Errorf("expected Field=Enabled, got %q", changes[0].Field)
	}
	if changes[0].OldValue != false || changes[0].NewValue != true {
		t.Errorf("expected false->true, got %v->%v", changes[0].OldValue, changes[0].NewValue)
	}
}

func TestReset_AfterChanges(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	p.Name = "jerry"
	p.Age = 25

	if !p.IsChanged() {
		t.Fatal("expected changes before Reset")
	}

	p.Reset()

	if p.IsChanged() {
		t.Error("expected no changes after Reset")
	}
	if len(p.Changes()) != 0 {
		t.Errorf("expected empty changes after Reset, got %d", len(p.Changes()))
	}

	p.Name = "new-name"
	if !p.IsChanged() {
		t.Error("expected changes after modifying post-Reset")
	}
	changes := p.Changes()
	if len(changes) != 1 || changes[0].Field != "Name" {
		t.Errorf("expected Name change after Reset, got %v", changes)
	}
}

func TestReset_NilWatcher(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil watcher")
		}
	}()

	var w *Watcher[Person]
	w.Reset()
}

func TestNew_MissingEmbeddedPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when T does not embed *Watcher[T]")
		}
	}()

	New(fakeWatchable{Name: "x"})
}

func TestNew_NonStructTypePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when T is an interface type")
		}
	}()

	New[Watchable](nil)
}

func TestChanges_InPlaceSliceMutation(t *testing.T) {
	c := New(Config{Tags: []string{"a", "b"}})

	c.Tags[0] = "x"

	changes := c.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Field != "Tags" {
		t.Errorf("expected Field=Tags, got %q", changes[0].Field)
	}
	wantOld := []string{"a", "b"}
	if !reflect.DeepEqual(changes[0].OldValue, wantOld) {
		t.Errorf("expected OldValue=%v, got %v", wantOld, changes[0].OldValue)
	}
	if !reflect.DeepEqual(changes[0].NewValue, []string{"x", "b"}) {
		t.Errorf("unexpected NewValue=%v", changes[0].NewValue)
	}
}

func TestChanges_InPlaceMapMutation(t *testing.T) {
	c := New(Config{M: map[string]int{"a": 1, "b": 2}})

	c.M["a"] = 100

	changes := c.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Field != "M" {
		t.Errorf("expected Field=M, got %q", changes[0].Field)
	}
	if !reflect.DeepEqual(changes[0].OldValue, map[string]int{"a": 1, "b": 2}) {
		t.Errorf("unexpected OldValue=%v", changes[0].OldValue)
	}
	if !reflect.DeepEqual(changes[0].NewValue, map[string]int{"a": 100, "b": 2}) {
		t.Errorf("unexpected NewValue=%v", changes[0].NewValue)
	}
}

func TestChanges_InPlacePointerMutation(t *testing.T) {
	n := 5
	b := New(Box{N: &n})

	*b.N = 6

	if !b.IsChanged() {
		t.Fatal("expected in-place pointer mutation to be detected")
	}
	changes := b.Changes()
	if len(changes) != 1 || changes[0].Field != "N" {
		t.Fatalf("expected single N change, got %v", changes)
	}
	if changes[0].OldValue != 5 || changes[0].NewValue != 6 {
		t.Errorf("expected 5->6, got %v->%v", changes[0].OldValue, changes[0].NewValue)
	}
}

func TestChanges_PointerSameValueNotChanged(t *testing.T) {
	n := 5
	b := New(Box{N: &n})

	m := 5
	b.N = &m // 指向不同地址但值相同，DeepEqual 语义下视为未变更

	if b.IsChanged() {
		t.Error("pointer to equal value should not be reported as changed")
	}
}

func TestReset_SnapshotIndependent(t *testing.T) {
	c := New(Config{Tags: []string{"a"}})

	c.Tags[0] = "b"
	c.Reset()

	c.Tags[0] = "c"

	changes := c.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change after in-place mutation post-Reset, got %d", len(changes))
	}
	if !reflect.DeepEqual(changes[0].OldValue, []string{"b"}) {
		t.Errorf("expected OldValue=[b], got %v", changes[0].OldValue)
	}
	if !reflect.DeepEqual(changes[0].NewValue, []string{"c"}) {
		t.Errorf("expected NewValue=[c], got %v", changes[0].NewValue)
	}
}

func TestChanges_NaNEqual(t *testing.T) {
	m := New(Metrics{Score: math.NaN()})

	if m.IsChanged() {
		t.Error("NaN field should not be reported as changed initially")
	}

	m.Reset()
	if m.IsChanged() {
		t.Error("NaN field should not be reported as changed after Reset")
	}

	m.Score = 1.5
	if !m.IsChanged() {
		t.Error("expected change from NaN to 1.5")
	}
}

func TestChanges_TimeEqual(t *testing.T) {
	now := time.Now()
	e := New(Event{At: now})

	e.At = now.Round(0) // 去掉单调时钟，时刻相同

	if e.IsChanged() {
		t.Error("time.Time with equal instants should not be reported as changed")
	}

	e.At = now.Add(time.Second)
	if !e.IsChanged() {
		t.Error("expected change for different time instant")
	}
}

func TestWatchTag_IgnoredFieldsNotReported(t *testing.T) {
	d := New(Document{Title: "t", Version: 1, Body: "b"})

	d.Title = "new-title"
	d.Version = 2
	d.Body = "new-body"

	changes := d.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (only untagged field), got %d: %v", len(changes), changes)
	}
	if changes[0].Field != "Body" {
		t.Errorf("expected Field=Body, got %q", changes[0].Field)
	}
}

func TestWatchTag_IgnoredFieldNotChanged(t *testing.T) {
	d := New(Document{Title: "t", Body: "b"})

	d.Title = "new-title"

	if d.IsChanged() {
		t.Error("ignored field modification should not count as change")
	}
}

func TestWatchTag_IgnoredRefFieldInPlaceMutation(t *testing.T) {
	d := New(Document{History: []string{"a"}, Reviewers: []string{"r"}})

	d.History[0] = "changed" // 被忽略的 slice 字段，原地修改也不检测

	if d.IsChanged() {
		t.Error("in-place mutation of ignored field should not count as change")
	}

	d.Reviewers[0] = "changed2" // 未忽略的字段需正常检测
	if !d.IsChanged() {
		t.Error("expected change on untagged field in-place mutation")
	}
}

func TestWatchTag_IgnoredAfterReset(t *testing.T) {
	d := New(Document{Title: "t", Version: 1, Body: "b"})

	d.Version = 2
	d.Reset()

	d.Title = "new-title"
	d.Version = 3

	if d.IsChanged() {
		t.Error("ignored fields should stay ignored across Reset")
	}
}

func TestWatchTag_InvalidTagPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on invalid watch tag value")
		}
		want := `structwatcher: invalid watch tag value "ignore" on field BadTag.Field (only "-" is supported)`
		if msg, ok := r.(string); !ok || msg != want {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()

	New(BadTag{Field: "x"})
}

func TestChanges_FuncField(t *testing.T) {
	fn := func() error { return nil }
	c := New(Callbacks{OnEvent: fn})

	// 同一 func：不误报（非 nil func 无法比较值相等，但指针一致）
	if c.IsChanged() {
		t.Error("same func value should not be reported as changed")
	}

	// 非 nil -> nil：报变更
	c.Reset()
	c.OnEvent = nil
	if !c.IsChanged() {
		t.Error("expected change from non-nil to nil func")
	}

	// nil -> 非 nil：报变更
	c.Reset()
	c.OnEvent = fn
	if !c.IsChanged() {
		t.Error("expected change from nil to non-nil func")
	}

	// Reset 后不再报（关键回归：旧实现会永久误报）
	c.Reset()
	if c.IsChanged() {
		t.Error("func field should not be reported as changed after Reset")
	}
}

func TestChanges_FuncNilToNil(t *testing.T) {
	c := New(Callbacks{})

	c.Reset()
	if c.IsChanged() {
		t.Error("nil->nil func should not be reported as changed")
	}
}

func TestChanges_ChanField(t *testing.T) {
	ch := make(chan int, 1)
	c := New(Channel{C: ch})

	// 同一 channel：不误报
	if c.IsChanged() {
		t.Error("same channel should not be reported as changed")
	}

	// 换成不同 channel：报变更
	c.Reset()
	c.C = make(chan int, 1)
	if !c.IsChanged() {
		t.Error("expected change when replacing channel")
	}

	// 非 nil -> nil：报变更
	c.Reset()
	c.C = nil
	if !c.IsChanged() {
		t.Error("expected change from non-nil to nil channel")
	}

	// nil -> 非 nil：报变更
	c.Reset()
	c.C = ch
	if !c.IsChanged() {
		t.Error("expected change from nil to non-nil channel")
	}
}

func TestChanges_ChanNilToNil(t *testing.T) {
	c := New(Channel{})

	c.Reset()
	if c.IsChanged() {
		t.Error("nil->nil channel should not be reported as changed")
	}
}

func TestHistory_Basic(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	p.Name = "jerry"
	p.Age = 25

	hist := p.History()
	if hist.Name != "tom" || hist.Age != 20 {
		t.Errorf("History got %q/%d, want tom/20", hist.Name, hist.Age)
	}
	if p.Name != "jerry" || p.Age != 25 {
		t.Errorf("History should not modify current value, got %q/%d", p.Name, p.Age)
	}
}

func TestHistory_AfterReset(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	p.Name = "jerry"
	p.Reset()

	hist := p.History()
	if hist.Name != "jerry" {
		t.Errorf("History after Reset got %q, want jerry", hist.Name)
	}
}

func TestHistory_RefFieldsDefensiveCopy(t *testing.T) {
	c := New(Config{Title: "t", Tags: []string{"a"}, M: map[string]int{"k": 1}})

	hist := c.History()
	hist.Tags[0] = "z"
	hist.M["k"] = 99

	if c.IsChanged() {
		t.Error("mutating History result should not mark struct changed")
	}

	c.Tags[0] = "b"
	changes := c.Changes()
	if len(changes) != 1 || changes[0].Field != "Tags" {
		t.Fatalf("expected only Tags change, got %v", changes)
	}
	if !reflect.DeepEqual(changes[0].OldValue, []string{"a"}) {
		t.Errorf("Tags OldValue affected by History mutation: %v", changes[0].OldValue)
	}
}

func TestHistory_IgnoredFieldsUseSnapshotValues(t *testing.T) {
	d := New(Document{Title: "v1", Body: "hello", History: []string{"a"}, Reviewers: []string{"r1"}})

	d.Title = "v2"

	// 注意：Document 自身的 History 字段会遮蔽方法名，需通过嵌入字段显式调用
	hist := d.Watcher.History()
	if hist.Title != "v1" {
		t.Errorf("ignored Title should keep snapshot value, got %q", hist.Title)
	}

	// 修改返回值中的忽略字段不应影响当前值
	hist.History[0] = "x"
	if d.History[0] != "a" {
		t.Errorf("mutating ignored field of History result affected target: %v", d.History)
	}
}

func TestHistory_EmbeddedWatcherPreserved(t *testing.T) {
	p := New(Person{Name: "tom", Age: 20})

	hist := p.History()
	if hist.Watcher != p.Watcher {
		t.Error("History result should keep the original embedded Watcher")
	}
}

func TestHistory_NilWatcher(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil watcher")
		}
	}()

	var w *Watcher[Person]
	w.History()
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New(Person{Name: "tom", Age: 20})
	}
}

func BenchmarkIsChanged_NoChange(b *testing.B) {
	p := New(Person{Name: "tom", Age: 20})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p.IsChanged() {
			b.Fatal("unexpected change")
		}
	}
}

func BenchmarkIsChanged_Changed(b *testing.B) {
	p := New(Person{Name: "tom", Age: 20})
	p.Name = "jerry"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !p.IsChanged() {
			b.Fatal("expected change")
		}
	}
}

func BenchmarkChanges(b *testing.B) {
	p := New(Person{Name: "tom", Age: 20})
	p.Name = "jerry"
	p.Age = 25
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Changes()
	}
}

func BenchmarkReset(b *testing.B) {
	p := New(Config{Title: "t", Tags: []string{"a", "b"}, M: map[string]int{"k": 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Reset()
	}
}
