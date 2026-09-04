package structwatcher

import (
	"reflect"
	"sync"
	"testing"
)

func TestSync_BasicFlow(t *testing.T) {
	s := NewSync(Person{Name: "tom", Age: 20})

	if s.IsChanged() {
		t.Error("newly created SyncWatcher should have no changes")
	}

	s.Update(func(p *Person) {
		p.Name = "jerry"
		p.Age = 25
	})

	if !s.IsChanged() {
		t.Fatal("expected changes after Update")
	}

	changes := s.Changes()
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	set := map[string]Change{}
	for _, c := range changes {
		set[c.Field] = c
	}
	if set["Name"].OldValue != "tom" || set["Name"].NewValue != "jerry" {
		t.Errorf("Name: expected tom->jerry, got %v->%v", set["Name"].OldValue, set["Name"].NewValue)
	}
	if set["Age"].OldValue != 20 || set["Age"].NewValue != 25 {
		t.Errorf("Age: expected 20->25, got %v->%v", set["Age"].OldValue, set["Age"].NewValue)
	}

	s.Reset()
	if s.IsChanged() {
		t.Error("expected no changes after Reset")
	}
}

func TestSync_UpdateNoModification(t *testing.T) {
	s := NewSync(Person{Name: "tom", Age: 20})

	s.Update(func(p *Person) {
		p.Name = "tom" // 赋相同值不算变更
	})

	if s.IsChanged() {
		t.Error("assigning equal value should not count as change")
	}
}

func TestSync_View(t *testing.T) {
	s := NewSync(Person{Name: "tom", Age: 20})

	var gotName string
	var gotAge int
	s.View(func(p *Person) {
		gotName, gotAge = p.Name, p.Age
	})

	if gotName != "tom" || gotAge != 20 {
		t.Errorf("View got %q/%d, want tom/20", gotName, gotAge)
	}
}

func TestSync_Unwrap(t *testing.T) {
	s := NewSync(Person{Name: "tom", Age: 20})

	raw := s.Unwrap()
	if raw.Name != "tom" || raw.Age != 20 {
		t.Errorf("Unwrap got %q/%d", raw.Name, raw.Age)
	}
}

func TestSync_ChangesStableAfterFurtherMutation(t *testing.T) {
	s := NewSync(Config{Tags: []string{"a"}, M: map[string]int{"k": 1}})

	s.Update(func(c *Config) {
		c.Tags = []string{"a", "b"}
		c.M = map[string]int{"k": 2}
	})

	changes := s.Changes()

	// 拿到结果后再原地修改当前值，返回的 NewValue 不应被影响
	s.Update(func(c *Config) {
		c.Tags[0] = "z"
		c.M["k"] = 99
	})

	set := map[string]Change{}
	for _, c := range changes {
		set[c.Field] = c
	}
	if !reflect.DeepEqual(set["Tags"].NewValue, []string{"a", "b"}) {
		t.Errorf("Tags NewValue mutated after return: %v", set["Tags"].NewValue)
	}
	if !reflect.DeepEqual(set["M"].NewValue, map[string]int{"k": 2}) {
		t.Errorf("M NewValue mutated after return: %v", set["M"].NewValue)
	}
}

func TestSync_History(t *testing.T) {
	s := NewSync(Person{Name: "tom", Age: 20})

	s.Update(func(p *Person) {
		p.Name = "jerry"
	})

	hist := s.History()
	if hist.Name != "tom" || hist.Age != 20 {
		t.Errorf("History got %q/%d, want tom/20", hist.Name, hist.Age)
	}
}

func TestSync_HistoryStableAfterFurtherMutation(t *testing.T) {
	s := NewSync(Config{Tags: []string{"a"}, M: map[string]int{"k": 1}})

	s.Update(func(c *Config) {
		c.Tags = []string{"a", "b"}
	})

	hist := s.History()

	// 拿到结果后再原地修改当前值，返回的历史对象不应被影响
	s.Update(func(c *Config) {
		c.Tags[0] = "z"
		c.M["k"] = 99
	})

	if !reflect.DeepEqual(hist.Tags, []string{"a"}) {
		t.Errorf("History Tags mutated after return: %v", hist.Tags)
	}
	if !reflect.DeepEqual(hist.M, map[string]int{"k": 1}) {
		t.Errorf("History M mutated after return: %v", hist.M)
	}
}

func TestSync_NilPanics(t *testing.T) {
	cases := map[string]func(){
		"Update":    func() { var s *SyncWatcher[Person]; s.Update(func(*Person) {}) },
		"View":      func() { var s *SyncWatcher[Person]; s.View(func(*Person) {}) },
		"IsChanged": func() { var s *SyncWatcher[Person]; s.IsChanged() },
		"Changes":   func() { var s *SyncWatcher[Person]; s.Changes() },
		"Reset":     func() { var s *SyncWatcher[Person]; s.Reset() },
		"History":   func() { var s *SyncWatcher[Person]; s.History() },
		"Unwrap":    func() { var s *SyncWatcher[Person]; s.Unwrap() },
	}
	for name, fn := range cases {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic on nil SyncWatcher for %s", name)
				}
			}()
			fn()
		}()
	}
}

func TestSync_Concurrent(t *testing.T) {
	s := NewSync(Config{Title: "t", Tags: []string{"a"}, M: map[string]int{"k": 1}})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				s.Update(func(c *Config) {
					c.Count = j
					c.Tags = append(c.Tags, "x")
				})
				_ = s.IsChanged()
				_ = s.Changes()
				if j%50 == 49 {
					s.Reset()
				}
				s.View(func(c *Config) {
					_ = c.Title
				})
			}
		}()
	}
	wg.Wait()
}

func BenchmarkSyncIsChanged_NoChange(b *testing.B) {
	s := NewSync(Person{Name: "tom", Age: 20})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.IsChanged() {
			b.Fatal("unexpected change")
		}
	}
}

func BenchmarkSyncUpdate(b *testing.B) {
	s := NewSync(Person{Name: "tom", Age: 20})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Update(func(p *Person) { p.Age = i })
	}
}
