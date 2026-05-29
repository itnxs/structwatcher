package structwatcher

import "testing"

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
	Count   int
	private string
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

// TestValidateWatchable_MissingEmbedded cannot be tested at runtime.
// Go's type system rejects types not implementing Watchable at compile time.
// The panic is enforced by the type constraint T Watchable in validateWatchable.
