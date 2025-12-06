package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bingo/engine"
	"bingo/internal/logger"

	"github.com/fsnotify/fsnotify"
)

// Reloader 规则文件热重载管理器
type Reloader struct {
	watcher   *fsnotify.Watcher
	filePath   string
	onReload   func(*engine.RulesConfig) error
	mu         sync.Mutex
	stopCh     chan struct{}
	debounceCh chan struct{}
	debounceDur time.Duration
}

// NewReloader 创建新的规则重载器
//
// :param filePath: 要监听的规则文件路径
// :param onReload: 重载回调函数，当文件变化时调用
// :param debounceDur: 防抖时间，避免频繁触发
func NewReloader(filePath string, onReload func(*engine.RulesConfig) error, debounceDur time.Duration) (*Reloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("创建文件监听器失败: %w", err)
	}

	// 获取文件的绝对路径
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		watcher.Close()
		return nil, fmt.Errorf("获取文件绝对路径失败: %w", err)
	}

	// 监听文件所在目录
	dir := filepath.Dir(absPath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("添加目录监听失败: %w", err)
	}

	return &Reloader{
		watcher:     watcher,
		filePath:    absPath,
		onReload:    onReload,
		stopCh:      make(chan struct{}),
		debounceCh:  make(chan struct{}, 1),
		debounceDur: debounceDur,
	}, nil
}

// Start 启动文件监听
func (r *Reloader) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	go r.watchLoop()
	logger.Infof("规则文件热重载已启动，监听文件: %s", r.filePath)
	return nil
}

// Stop 停止文件监听
func (r *Reloader) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	select {
	case <-r.stopCh:
		// 已经关闭
		return
	default:
		close(r.stopCh)
	}

	if r.watcher != nil {
		r.watcher.Close()
	}
	logger.Info("规则文件热重载已停止")
}

// watchLoop 监听循环
func (r *Reloader) watchLoop() {
	for {
		select {
		case <-r.stopCh:
			return
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			r.handleEvent(event)
		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			logger.Warnw("文件监听错误", "error", err)
		}
	}
}

// handleEvent 处理文件变化事件
func (r *Reloader) handleEvent(event fsnotify.Event) {
	// 只处理目标文件的变化
	if event.Name != r.filePath {
		return
	}

	// 处理文件删除事件
	if event.Op&fsnotify.Remove != 0 {
		logger.Warnf("规则文件被删除: %s，保持当前规则不变", r.filePath)
		return
	}

	// 只处理写入和重命名事件（文件被编辑或移动）
	if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Rename == 0 {
		return
	}

	// 防抖处理：避免频繁触发
	select {
	case r.debounceCh <- struct{}{}:
		// 启动防抖定时器
		go func() {
			time.Sleep(r.debounceDur)
			select {
			case <-r.debounceCh:
				// 执行重载
				r.reload()
			case <-r.stopCh:
				return
			}
		}()
	default:
		// 防抖通道已满，忽略此次事件
	}
}

// reload 执行规则重载
func (r *Reloader) reload() {
	logger.Info("检测到规则文件变化，开始重载...")

	// 检查文件是否存在
	if _, err := os.Stat(r.filePath); os.IsNotExist(err) {
		logger.Warnf("规则文件不存在: %s，保持当前规则不变", r.filePath)
		return
	}

	// 重新加载规则文件
	rulesConfig, err := engine.LoadRulesWithRedisConnections(r.filePath)
	if err != nil {
		logger.Errorw("重载规则文件失败，保持旧规则", "error", err, "file", r.filePath)
		return
	}

	// 调用回调函数
	if err := r.onReload(rulesConfig); err != nil {
		logger.Errorw("规则重载回调执行失败，保持旧规则", "error", err)
		return
	}

	logger.Infof("规则文件重载成功，共 %d 条规则", len(rulesConfig.Rules))
}

