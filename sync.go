package structwatcher

import (
	"reflect"
	"sync"
)

// SyncWatcher 是 Watcher 的并发安全包装，通过内部 RWMutex 保护
// 字段访问与变更检测，避免每个使用方自行加锁。
//
// 使用契约（库无法拦截对字段的直接赋值，违反契约仍会产生数据竞争）：
//   - 对字段的所有写操作必须通过 Update 进行；
//   - 并发读字段必须通过 View 进行；
//   - 不要在 Update/View 的回调内调用 SyncWatcher 自身或内嵌 Watcher
//     的任何方法（锁不可重入，会死锁）；
//   - 不要把回调传入的 *T 持有到回调之外。
//
// 确定无并发时，可通过 Unwrap 取回原始 *T 直接操作。
//
// Changes 返回的 NewValue 会做防御性深拷贝（仅引用类型），
// 保证返回结果不被后续并发修改影响。
type SyncWatcher[T Watchable] struct {
	mu sync.RWMutex
	t  *T
	w  *Watcher[T]
}

// NewSync 创建并初始化一个并发安全的被监听结构体，语义同 New。
func NewSync[T Watchable](initial T) *SyncWatcher[T] {
	t, w := newWatched(initial)
	return &SyncWatcher[T]{t: t, w: w}
}

// Update 在持有写锁的状态下执行 fn，fn 内可安全修改字段。
// fn 不得调用 SyncWatcher 或内嵌 Watcher 的任何方法（会死锁），
// 也不得将 *t 持有到回调之外。
// 如果接收器为 nil（未通过 NewSync 创建），则 panic。
func (s *SyncWatcher[T]) Update(fn func(t *T)) {
	if s == nil {
		panic("structwatcher: method Update called on nil SyncWatcher, use structwatcher.NewSync to create")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.t)
}

// View 在持有读锁的状态下执行 fn，fn 内可安全读取字段，但不得修改。
// fn 不得调用 SyncWatcher 或内嵌 Watcher 的任何方法（会死锁），
// 也不得将 *t 持有到回调之外。
// 如果接收器为 nil，则 panic。
func (s *SyncWatcher[T]) View(fn func(t *T)) {
	if s == nil {
		panic("structwatcher: method View called on nil SyncWatcher, use structwatcher.NewSync to create")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(s.t)
}

// IsChanged 返回是否有字段发生变更，语义同 Watcher.IsChanged。
// 如果接收器为 nil，则 panic。
func (s *SyncWatcher[T]) IsChanged() bool {
	if s == nil {
		panic("structwatcher: method IsChanged called on nil SyncWatcher, use structwatcher.NewSync to create")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.w.IsChanged()
}

// Changes 返回所有变更字段的列表，语义同 Watcher.Changes。
// 与 Watcher.Changes 的区别：返回的 NewValue 为防御性深拷贝，
// 不受后续并发修改影响。
// 如果接收器为 nil，则 panic。
func (s *SyncWatcher[T]) Changes() []Change {
	if s == nil {
		panic("structwatcher: method Changes called on nil SyncWatcher, use structwatcher.NewSync to create")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	changes := s.w.Changes()
	for i := range changes {
		changes[i].NewValue = stableValue(changes[i].NewValue)
	}
	return changes
}

// Reset 将当前值设为新的快照，语义同 Watcher.Reset。
// 如果接收器为 nil，则 panic。
func (s *SyncWatcher[T]) Reset() {
	if s == nil {
		panic("structwatcher: method Reset called on nil SyncWatcher, use structwatcher.NewSync to create")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w.Reset()
}

// Unwrap 返回被包装的原始 *T，供确定无并发访问时直接操作。
// 调用方需自行保证此后不再有并发的 Update/View/IsChanged/Changes/Reset，
// 否则会绕过锁产生数据竞争。
// 如果接收器为 nil，则 panic。
func (s *SyncWatcher[T]) Unwrap() *T {
	if s == nil {
		panic("structwatcher: method Unwrap called on nil SyncWatcher, use structwatcher.NewSync to create")
	}
	return s.t
}

// stableValue 返回值的一个稳定副本：引用语义类型深拷贝，
// 纯值类型原样返回（无需拷贝）。用于让 Changes 的结果
// 在锁释放后不被并发修改影响。
func stableValue(v any) any {
	if v == nil {
		return v
	}
	rv := reflect.ValueOf(v)
	if !cachedContainsRef(rv.Type()) {
		return v
	}
	return deepCopyValue(rv).Interface()
}
