package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"bingo/internal/logger"

	"github.com/redis/go-redis/v9"
)

// LockContent 锁内容结构
type LockContent struct {
	InstanceID string    `json:"instance_id"` // 实例ID（hostname-PID）
	Timestamp  int64     `json:"timestamp"`  // 时间戳
	AcquiredAt time.Time `json:"acquired_at"` // 获取锁的时间
}

// LockManager 分布式锁管理器
type LockManager struct {
	client        *redis.Client
	key           string
	ttl           time.Duration
	instanceID    string
	lockContent   *LockContent
	mu            sync.RWMutex
	isLeader      bool
	stopRefresh   chan struct{}
	stopValidate  chan struct{}
	refreshWg     sync.WaitGroup
	validateWg    sync.WaitGroup
}

// NewLockManager 创建新的锁管理器
func NewLockManager(client *redis.Client, key string, ttl time.Duration) (*LockManager, error) {
	if client == nil {
		return nil, fmt.Errorf("Redis 客户端不能为空")
	}

	// 生成实例ID：hostname-PID
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	instanceID := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	return &LockManager{
		client:      client,
		key:         key,
		ttl:         ttl,
		instanceID:  instanceID,
		stopRefresh: make(chan struct{}),
		stopValidate: make(chan struct{}),
	}, nil
}

// Acquire 尝试获取锁
func (lm *LockManager) Acquire(ctx context.Context) (bool, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// 创建锁内容
	lockContent := &LockContent{
		InstanceID: lm.instanceID,
		Timestamp:  time.Now().Unix(),
		AcquiredAt: time.Now(),
	}

	// 序列化锁内容
	contentBytes, err := json.Marshal(lockContent)
	if err != nil {
		return false, fmt.Errorf("序列化锁内容失败: %w", err)
	}

	// 使用 SET NX EX 命令获取锁
	result, err := lm.client.SetNX(ctx, lm.key, string(contentBytes), lm.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("获取锁失败: %w", err)
	}

	if result {
		// 成功获取锁
		lm.lockContent = lockContent
		lm.isLeader = true
		logger.Infow("成功获取分布式锁",
			"key", lm.key,
			"instance_id", lm.instanceID,
		)
		return true, nil
	}

	// 锁已被其他实例持有
	logger.Debugw("锁已被其他实例持有",
		"key", lm.key,
		"instance_id", lm.instanceID,
	)
	return false, nil
}

// Refresh 续期锁
func (lm *LockManager) Refresh(ctx context.Context) error {
	lm.mu.RLock()
	if !lm.isLeader || lm.lockContent == nil {
		lm.mu.RUnlock()
		return fmt.Errorf("当前实例不是 Leader，无法续期")
	}
	lockContent := lm.lockContent
	lm.mu.RUnlock()

	// 先读取锁内容，校验是否仍然属于当前实例
	currentContent, err := lm.getLockContent(ctx)
	if err != nil {
		return fmt.Errorf("读取锁内容失败: %w", err)
	}

	// 校验实例ID是否匹配
	if currentContent == nil || currentContent.InstanceID != lm.instanceID {
		// 锁已被其他实例获取
		lm.mu.Lock()
		lm.isLeader = false
		lm.lockContent = nil
		lm.mu.Unlock()
		return fmt.Errorf("锁已被其他实例获取，实例ID不匹配")
	}

	// 更新时间戳
	lockContent.Timestamp = time.Now().Unix()

	// 序列化锁内容
	contentBytes, err := json.Marshal(lockContent)
	if err != nil {
		return fmt.Errorf("序列化锁内容失败: %w", err)
	}

	// 续期锁（使用 SET EX 命令，覆盖现有值并设置新的过期时间）
	err = lm.client.Set(ctx, lm.key, string(contentBytes), lm.ttl).Err()
	if err != nil {
		return fmt.Errorf("续期锁失败: %w", err)
	}

	logger.Debugw("成功续期分布式锁",
		"key", lm.key,
		"instance_id", lm.instanceID,
		"ttl", lm.ttl,
	)
	return nil
}

// Validate 校验锁内容
func (lm *LockManager) Validate(ctx context.Context) (bool, error) {
	lm.mu.RLock()
	if !lm.isLeader {
		lm.mu.RUnlock()
		return false, nil
	}
	expectedInstanceID := lm.instanceID
	lm.mu.RUnlock()

	// 读取锁内容
	currentContent, err := lm.getLockContent(ctx)
	if err != nil {
		return false, fmt.Errorf("读取锁内容失败: %w", err)
	}

	// 如果锁不存在，说明锁已过期或被释放
	if currentContent == nil {
		lm.mu.Lock()
		lm.isLeader = false
		lm.lockContent = nil
		lm.mu.Unlock()
		logger.Warnw("锁已过期或被释放",
			"key", lm.key,
			"instance_id", lm.instanceID,
		)
		return false, nil
	}

	// 校验实例ID是否匹配
	if currentContent.InstanceID != expectedInstanceID {
		lm.mu.Lock()
		lm.isLeader = false
		lm.lockContent = nil
		lm.mu.Unlock()
		logger.Warnw("锁内容校验失败，实例ID不匹配",
			"key", lm.key,
			"expected_instance_id", expectedInstanceID,
			"actual_instance_id", currentContent.InstanceID,
		)
		return false, nil
	}

	// 校验通过
	return true, nil
}

// Release 释放锁
func (lm *LockManager) Release(ctx context.Context) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if !lm.isLeader {
		return nil // 不是 Leader，无需释放
	}

	// 先校验锁内容，确保锁仍然属于当前实例
	currentContent, err := lm.getLockContent(ctx)
	if err == nil && currentContent != nil && currentContent.InstanceID == lm.instanceID {
		// 只有锁仍然属于当前实例时才删除
		err = lm.client.Del(ctx, lm.key).Err()
		if err != nil {
			return fmt.Errorf("释放锁失败: %w", err)
		}
		logger.Infow("成功释放分布式锁",
			"key", lm.key,
			"instance_id", lm.instanceID,
		)
	}

	lm.isLeader = false
	lm.lockContent = nil
	return nil
}

// IsLeader 检查当前实例是否是 Leader
func (lm *LockManager) IsLeader() bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.isLeader
}

// StartRefresh 启动锁续期 goroutine
func (lm *LockManager) StartRefresh(ctx context.Context, interval time.Duration) {
	lm.refreshWg.Add(1)
	go func() {
		defer lm.refreshWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := lm.Refresh(ctx); err != nil {
					logger.Errorw("锁续期失败",
						"error", err,
						"key", lm.key,
						"instance_id", lm.instanceID,
					)
					// 续期失败，停止续期
					return
				}
			case <-lm.stopRefresh:
				logger.Debugw("停止锁续期",
					"key", lm.key,
					"instance_id", lm.instanceID,
				)
				return
			}
		}
	}()
}

// StopRefresh 停止锁续期
func (lm *LockManager) StopRefresh() {
	close(lm.stopRefresh)
	lm.refreshWg.Wait()
}

// StartValidate 启动锁校验 goroutine
func (lm *LockManager) StartValidate(ctx context.Context, interval time.Duration) {
	lm.validateWg.Add(1)
	go func() {
		defer lm.validateWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				valid, err := lm.Validate(ctx)
				if err != nil {
					logger.Errorw("锁校验失败",
						"error", err,
						"key", lm.key,
						"instance_id", lm.instanceID,
					)
					// 校验失败，停止校验
					return
				}
				if !valid {
					logger.Warnw("锁校验未通过，当前实例不再是 Leader",
						"key", lm.key,
						"instance_id", lm.instanceID,
					)
					// 校验未通过，停止校验
					return
				}
			case <-lm.stopValidate:
				logger.Debugw("停止锁校验",
					"key", lm.key,
					"instance_id", lm.instanceID,
				)
				return
			}
		}
	}()
}

// StopValidate 停止锁校验
func (lm *LockManager) StopValidate() {
	close(lm.stopValidate)
	lm.validateWg.Wait()
}

// getLockContent 从 Redis 读取锁内容
func (lm *LockManager) getLockContent(ctx context.Context) (*LockContent, error) {
	data, err := lm.client.Get(ctx, lm.key).Result()
	if err == redis.Nil {
		// 键不存在
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var content LockContent
	if err := json.Unmarshal([]byte(data), &content); err != nil {
		return nil, fmt.Errorf("解析锁内容失败: %w", err)
	}

	return &content, nil
}

// GetInstanceID 获取当前实例ID
func (lm *LockManager) GetInstanceID() string {
	return lm.instanceID
}

