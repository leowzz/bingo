package utils

import (
	"sync"
	"time"
)

// Debouncer 防抖器
type Debouncer struct {
	mu       sync.Mutex
	timers   map[string]*time.Timer
	duration time.Duration
}

// NewDebouncer 创建新的防抖器
func NewDebouncer(duration time.Duration) *Debouncer {
	return &Debouncer{
		timers:   make(map[string]*time.Timer),
		duration: duration,
	}
}

// Debounce 防抖执行函数
func (d *Debouncer) Debounce(key string, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 如果已存在定时器，先取消
	if timer, exists := d.timers[key]; exists {
		timer.Stop()
	}

	// 创建新的定时器
	d.timers[key] = time.AfterFunc(d.duration, func() {
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()
		fn()
	})
}

// BatchCollector 批量收集器
type BatchCollector struct {
	mu       sync.Mutex
	items    map[string][]interface{}
	duration time.Duration
	maxSize  int
	callback func(key string, items []interface{})
}

// NewBatchCollector 创建新的批量收集器
func NewBatchCollector(duration time.Duration, maxSize int, callback func(key string, items []interface{})) *BatchCollector {
	return &BatchCollector{
		items:    make(map[string][]interface{}),
		duration: duration,
		maxSize:  maxSize,
		callback: callback,
	}
}

// Add 添加项目到批量收集器
func (b *BatchCollector) Add(key string, item interface{}) {
	var shouldFlush bool
	var itemsToFlush []interface{}

	b.mu.Lock()
	// 初始化 key 的列表
	if b.items[key] == nil {
		b.items[key] = make([]interface{}, 0)
		// 设置定时器，在时间窗口后触发
		time.AfterFunc(b.duration, func() {
			b.flush(key)
		})
	}

	// 添加项目
	b.items[key] = append(b.items[key], item)

	// 如果达到最大大小，准备刷新（但不能在这里调用 flush，因为会死锁）
	if len(b.items[key]) >= b.maxSize {
		shouldFlush = true
		itemsToFlush = make([]interface{}, len(b.items[key]))
		copy(itemsToFlush, b.items[key])
		delete(b.items, key)
	}
	b.mu.Unlock()

	// 在锁外执行回调
	if shouldFlush && len(itemsToFlush) > 0 && b.callback != nil {
		b.callback(key, itemsToFlush)
	}
}

// flush 刷新指定 key 的数据
func (b *BatchCollector) flush(key string) {
	var items []interface{}

	b.mu.Lock()
	if itemsList, exists := b.items[key]; exists {
		items = make([]interface{}, len(itemsList))
		copy(items, itemsList)
		delete(b.items, key)
	}
	b.mu.Unlock()

	// 在锁外执行回调
	if len(items) > 0 && b.callback != nil {
		b.callback(key, items)
	}
}

// FlushAll 刷新所有数据
func (b *BatchCollector) FlushAll() {
	allItems := make(map[string][]interface{})

	b.mu.Lock()
	// 复制所有数据
	for k, items := range b.items {
		itemsCopy := make([]interface{}, len(items))
		copy(itemsCopy, items)
		allItems[k] = itemsCopy
	}
	// 清空所有数据
	b.items = make(map[string][]interface{})
	b.mu.Unlock()

	// 在锁外执行所有回调
	for key, items := range allItems {
		if len(items) > 0 && b.callback != nil {
			b.callback(key, items)
		}
	}
}
