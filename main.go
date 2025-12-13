package main

import (
	"context"
	"flag"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"bingo/config"
	"bingo/engine"
	"bingo/executor"
	"bingo/internal/ha"
	"bingo/internal/logger"
	"bingo/internal/metrics"
	"bingo/listener"
	"bingo/utils"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// App 应用主结构
type App struct {
	config       *config.Config
	matcher      *engine.Matcher
	executor     *executor.Executor
	listener     *listener.BinlogListener
	eventHandler *EventHandler
	reloader     *config.Reloader
	redisExec    *executor.RedisExecutor
	posStore     listener.PositionStore // 位置存储（用于启动时的位置加载）
	lockManager  *ha.LockManager        // 分布式锁管理器（用于高可用）
}

// NewApp 创建新的应用实例
func NewApp(cfgPath string) (*App, error) {
	// 加载配置
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}

	// 设置 GOMAXPROCS，避免在容器环境中误用过多 CPU 核心
	//
	// 问题背景：
	// 在 Docker/K8S 容器中，如果 Pod 没有配置 CPU Limit，Go 程序启动时会通过
	// runtime.NumCPU() 读取 /proc/cpuinfo，可能误认为拥有宿主机的全部核心（如 256 个）。
	// 这会导致 Go 运行时创建大量逻辑处理器（P）和系统线程（M），引发严重的性能问题：
	//
	// 1. 调度风暴：大量线程上下文切换导致 L1/L2 缓存命中率急剧下降
	// 2. 锁竞争恶化：256 个线程同时争抢共享资源（队列 Channel、规则 Map、内存分配器锁）
	// 3. GC 压力：P 数量越多，GC 协调开销越大，STW 时间显著增加
	// 4. 内存底噪：每个 P 的本地缓存（mcache）导致内存占用增加
	//
	// 对于 BinGo 这种 Binlog 处理引擎（串行顺序性要求高、IO 密集型、含锁竞争），
	// 必须通过显式设置 GOMAXPROCS 来限制并发度，避免性能倒退。
	//
	// 设置策略：WorkerPoolSize + 4
	// - WorkerPoolSize：工作池大小，用于处理 Binlog 事件
	// - +4：为系统 goroutine（如 GC、网络 I/O、信号处理等）预留额外容量
	// 与系统实际 CPU 核心数取最小值，避免配置错误导致设置过高
	actualCPU := runtime.NumCPU()
	desiredProcs := cfg.Performance.WorkerPoolSize + 4
	maxProcs := desiredProcs
	if maxProcs > actualCPU {
		maxProcs = actualCPU
	}
	runtime.GOMAXPROCS(maxProcs)

	// 初始化日志
	if err := logger.InitLogger(
		cfg.Logging.Level,
		cfg.Logging.Format,
		cfg.Logging.Output,
		cfg.Logging.File,
		cfg.Logging.MaxSize,
		cfg.Logging.MaxBackups,
		cfg.Logging.MaxAge,
		cfg.Logging.EnableGoroutineID, // 从配置文件读取
	); err != nil {
		return nil, err
	}
	defer logger.Sync()

	// 记录 GOMAXPROCS 设置信息
	if maxProcs < desiredProcs {
		logger.Infof("GOMAXPROCS 已设置为 %d（期望值: %d，系统 CPU 核心数: %d，已限制以避免超出系统能力）", maxProcs, desiredProcs, actualCPU)
	} else {
		logger.Infof("GOMAXPROCS 已设置为 %d（系统 CPU 核心数: %d，工作池大小: %d）", maxProcs, actualCPU, cfg.Performance.WorkerPoolSize)
	}

	// 加载规则和 Redis 连接配置
	rulesConfig, err := engine.LoadRulesWithRedisConnections(cfg.RulesFile)
	if err != nil {
		return nil, err
	}

	// 从规则中提取需要监控的表列表
	monitoredTables := engine.ExtractMonitoredTables(rulesConfig.Rules)
	logger.Infof("从规则中解析出 %d 个监控表", len(monitoredTables))
	for _, table := range monitoredTables {
		logger.Infof("  监控表: %s", table)
	}

	// 创建匹配器
	matcher := engine.NewMatcher(rulesConfig.Rules)

	// 创建执行器
	exec := executor.NewExecutor()

	// 初始化系统 Redis 客户端（用于位置存储）
	var systemRedisClient *redis.Client
	if cfg.SystemRedis.Addr != "" {
		systemRedisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.SystemRedis.Addr,
			Username: cfg.SystemRedis.Username, // Redis 6.0+ ACL 支持
			Password: cfg.SystemRedis.Password,
			DB:       cfg.SystemRedis.DB,
		})

		// 测试 Redis 连接
		ctx := context.Background()
		if err := systemRedisClient.Ping(ctx).Err(); err != nil {
			logger.Warnf("系统 Redis 连接失败: %v", err)
			systemRedisClient = nil
		} else {
			logger.Info("系统 Redis 连接成功")
		}
	}

	// 初始化 Redis 执行器
	redisExec := executor.NewRedisExecutor()

	// 添加系统 Redis 连接（如果配置了），规则可以通过 "system" 名称使用
	if cfg.SystemRedis.Addr != "" && systemRedisClient != nil {
		// 直接使用已创建的客户端，避免重复创建
		redisExec.AddConnectionWithClient("system", systemRedisClient)
		logger.Info("系统 Redis 连接已添加到执行器（规则可通过 'system' 名称使用）")
	}

	// 添加规则中配置的 Redis 连接
	if len(rulesConfig.RedisConnections) > 0 {
		for _, conn := range rulesConfig.RedisConnections {
			// 检查是否与系统 Redis 冲突
			if conn.Name == "system" {
				logger.Warnf("规则 Redis 连接名称 'system' 与系统 Redis 冲突，将跳过")
				continue
			}
			if err := redisExec.AddConnection(conn.Name, conn.Addr, conn.Username, conn.Password, conn.DB); err != nil {
				logger.Warnf("添加 Redis 连接失败 [%s]: %v", conn.Name, err)
			} else {
				logger.Infof("Redis 连接已添加: %s", conn.Name)
			}
		}
	}

	// 如果没有配置任何连接，但配置了系统 Redis，使用系统 Redis 作为默认连接
	if !redisExec.HasConnections() && cfg.SystemRedis.Addr != "" && systemRedisClient != nil {
		redisExec.AddConnectionWithClient("default", systemRedisClient)
		logger.Info("使用系统 Redis 作为默认连接")
	}

	// 注册 Redis 执行器
	if redisExec.HasConnections() {
		exec.Register(redisExec)
		logger.Info("Redis 执行器已注册")
	}

	// 创建 Binlog 监听器配置
	canalCfg := canal.NewDefaultConfig()
	canalCfg.Addr = cfg.MySQL.Addr()
	canalCfg.User = cfg.MySQL.User
	canalCfg.Password = cfg.MySQL.Password
	canalCfg.Dump.TableDB = ""
	canalCfg.Dump.Tables = nil
	canalCfg.Dump.ExecutionPath = ""

	// 创建事件处理器（异步处理）
	eventHandler := NewEventHandler(
		matcher,
		exec,
		cfg.Performance.QueueSize,
		cfg.Performance.WorkerPoolSize,
	)

	// 创建位置存储（如果配置了系统 Redis 且启用了位置存储）
	var posStore listener.PositionStore
	if cfg.Binlog.UseRedisStore && systemRedisClient != nil {
		posStore = listener.NewRedisPositionStore(systemRedisClient, cfg.Binlog.RedisStoreKey)
		logger.Infof("已启用 Redis 位置存储，键名: %s", cfg.Binlog.RedisStoreKey)
	}

	// 创建监听器，使用从规则中解析出的监控表列表
	// 配置位置保存间隔和是否在事务提交时保存
	saveInterval := time.Duration(cfg.Binlog.SaveInterval) * time.Second
	if saveInterval == 0 {
		saveInterval = 5 * time.Second // 默认 5 秒
	}
	binlogListener, err := listener.New(listener.Config{
		CanalCfg:          canalCfg,
		Handler:           eventHandler,
		Tables:            monitoredTables,
		PosStore:          posStore,
		SaveInterval:      saveInterval,
		SaveOnTransaction: cfg.Binlog.SaveOnTransaction,
	})
	if err != nil {
		return nil, err
	}

	// 初始化分布式锁管理器（如果启用高可用）
	var lockManager *ha.LockManager
	if cfg.HA.Enabled && systemRedisClient != nil {
		lockTTL := time.Duration(cfg.HA.LockTTL) * time.Second
		lm, err := ha.NewLockManager(systemRedisClient, cfg.HA.LockKey, lockTTL)
		if err != nil {
			logger.Warnf("初始化分布式锁管理器失败: %v，将禁用高可用", err)
		} else {
			lockManager = lm
			logger.Infof("已初始化分布式锁管理器，锁键: %s, TTL: %d秒", cfg.HA.LockKey, cfg.HA.LockTTL)
		}
	} else if cfg.HA.Enabled && systemRedisClient == nil {
		logger.Warnf("高可用已启用，但系统 Redis 未配置或连接失败，将禁用高可用")
	}

	app := &App{
		config:       cfg,
		matcher:      matcher,
		executor:     exec,
		listener:     binlogListener,
		eventHandler: eventHandler,
		reloader:     nil, // 将在下面初始化
		redisExec:    redisExec,
		posStore:     posStore,
		lockManager:  lockManager,
	}

	// 启动事件处理器工作池
	eventHandler.Start()

	// 初始化并启动规则文件热重载
	reloader, err := config.NewReloader(
		cfg.RulesFile,
		app.reloadRules,
		500*time.Millisecond, // 防抖时间 500ms
	)
	if err != nil {
		logger.Warnf("初始化规则文件热重载失败: %v", err)
	} else {
		app.reloader = reloader
		if err := reloader.Start(); err != nil {
			logger.Warnf("启动规则文件热重载失败: %v", err)
		}
	}

	return app, nil
}

// Start 启动应用
func (a *App) Start() error {
	logger.Info("正在启动 BinGo 服务...")
	logger.Infof("MySQL: %s", a.config.MySQL.Addr())
	logger.Infof("规则文件: %s", a.config.RulesFile)
	logger.Infof("已加载 %d 条规则", len(a.matcher.GetRules()))

	// 启动 Prometheus metrics 端点
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		metricsAddr := ":9090"
		logger.Infof("Prometheus metrics 端点启动在 %s/metrics", metricsAddr)
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			logger.Warnf("启动 metrics 端点失败: %v", err)
		}
	}()

	// 设置信号处理，优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("正在停止服务...")
		a.Stop()
		os.Exit(0)
	}()

	// 如果启用高可用，先尝试获取锁
	if a.lockManager != nil {
		ctx := context.Background()
		acquired, err := a.lockManager.Acquire(ctx)
		if err != nil {
			logger.Errorw("获取分布式锁失败", "error", err)
			return fmt.Errorf("获取分布式锁失败: %w", err)
		}

		if !acquired {
			// 未获取到锁，进入 standby 模式
			logger.Infow("未获取到分布式锁，进入 Standby 模式，将定期重试",
				"instance_id", a.lockManager.GetInstanceID(),
			)
			return a.startStandbyMode()
		}

		// 成功获取锁，启动锁续期和校验机制
		refreshInterval := time.Duration(a.config.HA.RefreshInterval) * time.Second
		validateInterval := refreshInterval / 2 // 校验间隔为续期间隔的一半
		if validateInterval < 2*time.Second {
			validateInterval = 2 * time.Second // 最小 2 秒
		}

		ctx = context.Background()
		a.lockManager.StartRefresh(ctx, refreshInterval)
		a.lockManager.StartValidate(ctx, validateInterval)

		logger.Infow("已启动锁续期和校验机制",
			"refresh_interval", refreshInterval,
			"validate_interval", validateInterval,
			"instance_id", a.lockManager.GetInstanceID(),
		)

		// 启动监控 goroutine，检查锁状态
		go a.monitorLockStatus()

		// 启动 binlog 监听
		return a.startBinlogListener()
	}

	// 未启用高可用，直接启动 binlog 监听
	return a.startBinlogListener()
}

// reloadRules 重载规则的回调函数
func (a *App) reloadRules(rulesConfig *engine.RulesConfig) error {
	logger.Info("开始重载规则...")

	// 更新匹配器的规则
	a.matcher.UpdateRules(rulesConfig.Rules)
	logger.Infof("已更新 %d 条规则", len(rulesConfig.Rules))

	// 更新事件处理器的规则相关资源（batch collectors 和 ordering shards）
	if a.eventHandler != nil {
		a.eventHandler.UpdateRules(rulesConfig.Rules)
	}

	// 更新 Redis 连接配置
	if a.redisExec != nil {
		if err := a.redisExec.UpdateConnections(rulesConfig.RedisConnections); err != nil {
			logger.Warnw("更新 Redis 连接配置失败", "error", err)
			// 不返回错误，因为规则已经更新成功
		}
	}

	logger.Info("规则重载完成")
	return nil
}

// startStandbyMode 启动 Standby 模式，定期重试获取锁
func (a *App) startStandbyMode() error {
	logger.Info("进入 Standby 模式，等待获取分布式锁...")

	// 定期重试获取锁
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	ctx := context.Background()
	for {
		select {
		case <-ticker.C:
			acquired, err := a.lockManager.Acquire(ctx)
			if err != nil {
				logger.Warnw("尝试获取分布式锁失败", "error", err)
				continue
			}

			if acquired {
				// 成功获取锁，启动服务
				logger.Infow("成功获取分布式锁，开始启动服务",
					"instance_id", a.lockManager.GetInstanceID(),
				)

				// 如果 listener 已被关闭，需要重新创建
				if a.listener == nil {
					if err := a.recreateListener(); err != nil {
						logger.Errorw("重新创建 Binlog 监听器失败", "error", err)
						// 释放锁，等待下次重试
						_ = a.lockManager.Release(ctx)
						continue
					}
				}

				// 启动锁续期和校验机制
				refreshInterval := time.Duration(a.config.HA.RefreshInterval) * time.Second
				validateInterval := refreshInterval / 2
				if validateInterval < 2*time.Second {
					validateInterval = 2 * time.Second
				}

				a.lockManager.StartRefresh(ctx, refreshInterval)
				a.lockManager.StartValidate(ctx, validateInterval)

				// 启动监控 goroutine
				go a.monitorLockStatus()

				// 启动 binlog 监听
				return a.startBinlogListener()
			}
		}
	}
}

// recreateListener 重新创建 Binlog 监听器
func (a *App) recreateListener() error {
	// 从规则中提取需要监控的表列表
	monitoredTables := engine.ExtractMonitoredTables(a.matcher.GetRules())

	// 创建 Binlog 监听器配置
	canalCfg := canal.NewDefaultConfig()
	canalCfg.Addr = a.config.MySQL.Addr()
	canalCfg.User = a.config.MySQL.User
	canalCfg.Password = a.config.MySQL.Password
	canalCfg.Dump.TableDB = ""
	canalCfg.Dump.Tables = nil
	canalCfg.Dump.ExecutionPath = ""

	// 配置位置保存间隔
	saveInterval := time.Duration(a.config.Binlog.SaveInterval) * time.Second
	if saveInterval == 0 {
		saveInterval = 5 * time.Second
	}

	// 重新创建监听器
	binlogListener, err := listener.New(listener.Config{
		CanalCfg:          canalCfg,
		Handler:           a.eventHandler,
		Tables:            monitoredTables,
		PosStore:          a.posStore,
		SaveInterval:      saveInterval,
		SaveOnTransaction: a.config.Binlog.SaveOnTransaction,
	})
	if err != nil {
		return fmt.Errorf("重新创建 Binlog 监听器失败: %w", err)
	}

	a.listener = binlogListener
	logger.Info("已重新创建 Binlog 监听器")
	return nil
}

// startBinlogListener 启动 Binlog 监听（从位置存储或配置中加载位置）
func (a *App) startBinlogListener() error {
	// 启动监听 - 实现位置优先级逻辑
	// 优先级：1. Redis位置 2. 配置文件位置（并保存到Redis） 3. 当前binlog位置
	if a.config.Binlog.UseRedisStore && a.posStore != nil {
		// 如果启用了Redis存储，优先从Redis加载位置
		ctx := context.Background()
		redisPos, err := a.posStore.Load(ctx)
		if err != nil {
			logger.Warnw("从 Redis 加载位置失败，将尝试其他方式", "error", err)
		} else if redisPos != nil {
			// Redis中有位置，使用Redis位置
			logger.Infow("从 Redis 加载 Binlog 位置", "file", redisPos.Name, "position", redisPos.Pos)
			return a.listener.StartFromPosition(redisPos.Name, redisPos.Pos)
		}
		// Redis中没有位置，继续检查配置文件
	}

	// 检查配置文件中的位置
	if a.config.Binlog.File != "" {
		logger.Infof("从配置文件指定的 Binlog 位置: %s:%d 开始监听", a.config.Binlog.File, a.config.Binlog.Position)
		// 如果启用了Redis存储，将配置文件的位置保存到Redis
		if a.config.Binlog.UseRedisStore && a.posStore != nil {
			pos := mysql.Position{
				Name: a.config.Binlog.File,
				Pos:  a.config.Binlog.Position,
			}
			ctx := context.Background()
			if err := a.posStore.Save(ctx, pos); err != nil {
				logger.Warnw("保存配置文件位置到 Redis 失败", "error", err, "file", pos.Name, "position", pos.Pos)
			} else {
				logger.Infow("已将配置文件位置保存到 Redis", "file", pos.Name, "position", pos.Pos)
			}
		}
		return a.listener.StartFromPosition(a.config.Binlog.File, a.config.Binlog.Position)
	}

	// 配置文件也没有位置，从当前binlog位置开始
	logger.Info("从当前位置开始监听 Binlog 变更...")
	return a.listener.Start()
}

// monitorLockStatus 监控锁状态，如果失去锁则停止 binlog 监听
func (a *App) monitorLockStatus() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if a.lockManager == nil {
				return
			}

			// 检查是否是 Leader
			if !a.lockManager.IsLeader() {
				logger.Warnw("当前实例不再是 Leader，停止 Binlog 监听",
					"instance_id", a.lockManager.GetInstanceID(),
				)

				// 停止 binlog 监听（关闭后无法重启，需要在重新获取锁后重新创建）
				if a.listener != nil {
					a.listener.Close()
					a.listener = nil
				}

				// 进入 Standby 模式（在后台运行，不会阻塞）
				go func() {
					if err := a.startStandbyMode(); err != nil {
						logger.Errorw("Standby 模式启动失败", "error", err)
					}
				}()

				return
			}
		}
	}
}

// Stop 停止应用
func (a *App) Stop() {
	// 停止规则文件热重载
	if a.reloader != nil {
		a.reloader.Stop()
	}

	// 先停止事件处理器，等待工作池完成当前任务
	if a.eventHandler != nil {
		a.eventHandler.Stop()
	}

	// 然后关闭 binlog 监听器
	if a.listener != nil {
		a.listener.Close()
	}

	// 停止锁续期和校验，并释放锁
	if a.lockManager != nil {
		a.lockManager.StopRefresh()
		a.lockManager.StopValidate()
		ctx := context.Background()
		if err := a.lockManager.Release(ctx); err != nil {
			logger.Warnw("释放分布式锁失败", "error", err)
		}
	}

	logger.Info("服务已停止")
	logger.Sync()
}

// EventHandler 事件处理器
type EventHandler struct {
	matcher    *engine.Matcher
	executor   *executor.Executor
	eventQueue chan *listener.Event
	workers    int
	wg         sync.WaitGroup
	stopChan   chan struct{}

	// Batch 支持：规则ID -> BatchCollector
	batchCollectors map[string]*utils.BatchCollector
	batchMu         sync.RWMutex

	// Ordering 支持：规则ID -> 分片队列数组
	orderingShards map[string][]chan *listener.Event
	orderingMu     sync.RWMutex
}

// NewEventHandler 创建新的事件处理器
func NewEventHandler(matcher *engine.Matcher, executor *executor.Executor, queueSize, workerPoolSize int) *EventHandler {
	return &EventHandler{
		matcher:         matcher,
		executor:        executor,
		eventQueue:      make(chan *listener.Event, queueSize),
		workers:         workerPoolSize,
		stopChan:        make(chan struct{}),
		batchCollectors: make(map[string]*utils.BatchCollector),
		orderingShards:  make(map[string][]chan *listener.Event),
	}
}

// Start 启动工作池
func (h *EventHandler) Start() {
	logger.Infof("启动事件处理器工作池，工作线程数: %d, 队列大小: %d", h.workers, cap(h.eventQueue))
	for i := 0; i < h.workers; i++ {
		h.wg.Add(1)
		go h.worker(i)
	}
}

// Stop 停止工作池（优雅关闭，等待所有事件处理完成）
func (h *EventHandler) Stop() {
	logger.Info("正在停止事件处理器工作池...")

	// 先关闭 stopChan，通知 worker 准备停止（不再接受新事件入队）
	// 同时 OnEvent 也会检测到 stopChan 关闭，会同步处理事件而不是入队
	close(h.stopChan)

	// 刷新所有 batch collectors
	h.batchMu.Lock()
	for ruleID, collector := range h.batchCollectors {
		logger.Infow("刷新批量收集器", "rule_id", ruleID)
		collector.FlushAll()
	}
	h.batchMu.Unlock()

	// 关闭所有 ordering shard 队列
	h.orderingMu.Lock()
	for ruleID, shards := range h.orderingShards {
		logger.Infow("关闭顺序保障分片队列", "rule_id", ruleID, "shard_count", len(shards))
		for _, shard := range shards {
			close(shard)
		}
	}
	h.orderingMu.Unlock()

	// 等待所有 worker 处理完队列中的剩余事件
	logger.Info("等待工作线程处理完队列中的事件...")
	h.wg.Wait()

	// 所有 worker 都已退出，队列中的事件都已处理完
	// 现在可以安全关闭队列（虽然 worker 已退出，但为了清理资源仍然关闭）
	close(h.eventQueue)

	logger.Info("事件处理器工作池已停止")
}

// OnEvent 处理事件（异步入队）
func (h *EventHandler) OnEvent(event *listener.Event) error {
	// 记录事件（用于统计）
	metrics.RecordEvent(event.Table, string(event.Action))

	// 将事件加入队列（阻塞等待，确保不丢失事件）
	select {
	case h.eventQueue <- event:
		return nil
	case <-h.stopChan:
		// 正在关闭，但仍尝试处理当前事件
		// 由于正在关闭，可以选择同步处理或者记录日志
		logger.Warnw("事件处理器正在关闭，但会处理当前事件", "table", event.Table, "action", event.Action)
		// 同步处理当前事件，确保不丢失
		h.processEvent(event)
		return nil
	}
}

// worker 工作线程，处理事件队列中的事件
func (h *EventHandler) worker(id int) {
	defer h.wg.Done()
	logger.Debugw("事件处理工作线程启动", "worker_id", id)

	for {
		select {
		case event, ok := <-h.eventQueue:
			if !ok {
				// 队列已关闭，退出
				logger.Debugw("事件处理工作线程退出（队列已关闭）", "worker_id", id)
				return
			}
			h.processEvent(event)
		case <-h.stopChan:
			// 收到停止信号，处理完队列中的所有剩余事件后退出
			logger.Debugw("事件处理工作线程收到停止信号，处理剩余事件", "worker_id", id)
			// 继续处理队列中的所有剩余事件，确保不丢失
			for {
				select {
				case event, ok := <-h.eventQueue:
					if !ok {
						// 队列已关闭，退出
						logger.Debugw("事件处理工作线程退出（队列已关闭）", "worker_id", id)
						return
					}
					h.processEvent(event)
				default:
					// 队列已空，退出
					logger.Debugw("事件处理工作线程退出（队列已空）", "worker_id", id)
					return
				}
			}
		}
	}
}

// processEvent 处理单个事件（原来的同步处理逻辑）
func (h *EventHandler) processEvent(event *listener.Event) {
	// 注意：对于 batch 和 ordering 模式，事件会在异步处理完成后归还
	// 对于普通模式，在函数结束时归还
	needReturnToPool := true
	defer func() {
		if needReturnToPool {
			listener.PutEventToPool(event)
		}
	}()

	startTime := time.Now()

	// Debug: 打印收到的事件信息
	logger.Infow("收到事件",
		"table", event.Table,
		"action", event.Action,
		"schema", event.Schema,
		"table_name", event.TableName,
		"timestamp", event.Timestamp,
		"has_old_row", event.OldRow != nil,
		"has_new_row", event.NewRow != nil,
	)

	// 如果有数据，打印详细内容（Debug 级别）
	if event.NewRow != nil {
		logger.Debugw("事件数据 (NewRow)", "data", event.NewRow)
	}
	if event.OldRow != nil {
		logger.Debugw("事件数据 (OldRow)", "data", event.OldRow)
	}

	// 匹配规则
	matchedRules, err := h.matcher.Match(event)
	if err != nil {
		logger.Errorw("规则匹配失败", "error", err, "table", event.Table, "action", event.Action)
		return
	}

	if len(matchedRules) == 0 {
		// 没有匹配的规则，直接返回
		logger.Debugw("未匹配到规则", "table", event.Table, "action", event.Action)
		return
	}

	logger.Infow("规则匹配成功",
		"table", event.Table,
		"action", event.Action,
		"matched_count", len(matchedRules),
		"rule_ids", func() []string {
			ids := make([]string, len(matchedRules))
			for i, r := range matchedRules {
				ids[i] = r.ID
			}
			return ids
		}(),
	)

	// 处理每个匹配的规则
	hasAsyncProcessing := false
	for _, rule := range matchedRules {
		// 检查是否需要批量处理
		if rule.Batch != nil && rule.Batch.Enabled {
			h.handleBatchRule(rule, event)
			hasAsyncProcessing = true
			continue
		}

		// 检查是否需要顺序保障
		if rule.Ordering != nil && rule.Ordering.Enabled {
			h.handleOrderingRule(rule, event)
			hasAsyncProcessing = true
			continue
		}

		// 普通处理：立即执行
		h.executeRule(rule, event, startTime)
	}

	// 如果有异步处理，不在这里归还事件（会在异步处理完成后归还）
	if hasAsyncProcessing {
		needReturnToPool = false
	}

	// 记录总处理耗时
	totalDuration := time.Since(startTime).Seconds()
	metrics.RecordProcessingDuration("total", "event", totalDuration)
}

// handleBatchRule 处理启用批量聚合的规则
func (h *EventHandler) handleBatchRule(rule engine.Rule, event *listener.Event) {
	h.batchMu.RLock()
	collector, exists := h.batchCollectors[rule.ID]
	h.batchMu.RUnlock()

	if !exists {
		// 创建新的 batch collector
		window := time.Duration(rule.Batch.Window) * time.Millisecond
		if window == 0 {
			window = 100 * time.Millisecond // 默认 100ms
		}
		maxSize := rule.Batch.MaxSize
		if maxSize == 0 {
			maxSize = 1000 // 默认 1000
		}

		collector = utils.NewBatchCollector(window, maxSize, func(key string, items []interface{}) {
			// 批量执行回调
			h.executeBatchActions(rule, items)
		})

		h.batchMu.Lock()
		h.batchCollectors[rule.ID] = collector
		h.batchMu.Unlock()

		logger.Infow("创建批量收集器",
			"rule_id", rule.ID,
			"window_ms", rule.Batch.Window,
			"max_size", maxSize,
		)
	}

	// 生成 batch key（基于表名和动作类型）
	batchKey := fmt.Sprintf("%s:%s", event.Table, event.Action)
	collector.Add(batchKey, event)
}

// handleOrderingRule 处理启用顺序保障的规则
func (h *EventHandler) handleOrderingRule(rule engine.Rule, event *listener.Event) {
	// 获取主键值
	keyValue := event.GetFieldString(rule.Ordering.KeyField)
	if keyValue == "" {
		logger.Warnw("顺序保障规则缺少主键字段值，使用普通处理",
			"rule_id", rule.ID,
			"key_field", rule.Ordering.KeyField,
		)
		// 注意：这里直接执行，事件会在 processEvent 中归还
		// 所以不需要特殊处理
		h.executeRule(rule, event, time.Now())
		return
	}

	// 计算分片索引
	shardIndex := h.calculateShard(keyValue, rule.Ordering.Shards)

	// 获取或创建分片队列
	h.orderingMu.RLock()
	shards, exists := h.orderingShards[rule.ID]
	h.orderingMu.RUnlock()

	if !exists {
		// 创建分片队列
		shardCount := rule.Ordering.Shards
		if shardCount == 0 {
			shardCount = 10 // 默认 10 个分片
		}
		shards = make([]chan *listener.Event, shardCount)
		// 保存规则引用，避免值传递导致的问题
		rulePtr := &rule
		for i := 0; i < shardCount; i++ {
			shards[i] = make(chan *listener.Event, cap(h.eventQueue))
			// 启动分片处理 goroutine
			go h.orderingShardWorker(rulePtr, i, shards[i])
		}

		h.orderingMu.Lock()
		h.orderingShards[rule.ID] = shards
		h.orderingMu.Unlock()

		logger.Infow("创建顺序保障分片队列",
			"rule_id", rule.ID,
			"shard_count", shardCount,
		)
	}

	// 将事件发送到对应的分片队列
	select {
	case shards[shardIndex] <- event:
		// 成功入队
	case <-h.stopChan:
		// 正在关闭，同步处理
		h.executeRule(rule, event, time.Now())
	default:
		// 队列满，同步处理（避免阻塞）
		logger.Warnw("顺序保障分片队列已满，同步处理",
			"rule_id", rule.ID,
			"shard_index", shardIndex,
		)
		h.executeRule(rule, event, time.Now())
	}
}

// orderingShardWorker 顺序保障分片工作线程
func (h *EventHandler) orderingShardWorker(rule *engine.Rule, shardIndex int, shardQueue chan *listener.Event) {
	for {
		select {
		case event, ok := <-shardQueue:
			if !ok {
				return
			}
			h.executeRule(*rule, event, time.Now())
		case <-h.stopChan:
			// 处理剩余事件
			for {
				select {
				case event, ok := <-shardQueue:
					if !ok {
						return
					}
					h.executeRule(*rule, event, time.Now())
				default:
					return
				}
			}
		}
	}
}

// calculateShard 计算分片索引
func (h *EventHandler) calculateShard(keyValue string, shardCount int) int {
	if shardCount <= 0 {
		shardCount = 10
	}
	// 使用 FNV-1a hash 算法
	hash := fnv.New32a()
	hash.Write([]byte(keyValue))
	return int(hash.Sum32()) % shardCount
}

// executeBatchActions 批量执行动作
func (h *EventHandler) executeBatchActions(rule engine.Rule, events []interface{}) {
	if len(events) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 将事件列表转换为 []*listener.Event
	eventList := make([]*listener.Event, 0, len(events))
	for _, item := range events {
		if evt, ok := item.(*listener.Event); ok {
			eventList = append(eventList, evt)
		}
	}

	logger.Infow("批量执行规则",
		"rule_id", rule.ID,
		"batch_size", len(eventList),
	)

	// 批量执行每个动作
	for _, action := range rule.Actions {
		// 注意：这里需要执行器支持批量操作
		// 目前先逐个执行，后续可以优化为真正的批量操作
		for _, event := range eventList {
			if err := h.executor.Execute(ctx, action, event); err != nil {
				logger.Errorw("批量执行动作失败",
					"rule_id", rule.ID,
					"action_type", action.Type,
					"error", err,
				)
				metrics.RecordActionFailed(action.Type, rule.ID, "batch_execution_error")
			} else {
				metrics.RecordActionExecuted(action.Type, rule.ID)
			}
		}
	}

	// 归还事件到对象池
	for _, event := range eventList {
		listener.PutEventToPool(event)
	}
}

// UpdateRules 更新规则（用于热重载）
func (h *EventHandler) UpdateRules(rules []engine.Rule) {
	// 清理旧的 batch collectors 和 ordering shards
	h.batchMu.Lock()
	// 获取当前规则ID集合
	currentRuleIDs := make(map[string]bool)
	for _, rule := range rules {
		currentRuleIDs[rule.ID] = true
	}
	// 删除不存在的规则对应的 collectors
	for ruleID, collector := range h.batchCollectors {
		if !currentRuleIDs[ruleID] {
			collector.FlushAll()
			delete(h.batchCollectors, ruleID)
			logger.Infow("删除批量收集器", "rule_id", ruleID)
		}
	}
	h.batchMu.Unlock()

	h.orderingMu.Lock()
	// 删除不存在的规则对应的 shards
	for ruleID, shards := range h.orderingShards {
		if !currentRuleIDs[ruleID] {
			for _, shard := range shards {
				close(shard)
			}
			delete(h.orderingShards, ruleID)
			logger.Infow("删除顺序保障分片队列", "rule_id", ruleID)
		}
	}
	h.orderingMu.Unlock()

	// 新的规则会在首次匹配时自动创建对应的 collectors 和 shards
}

// executeRule 执行单个规则
func (h *EventHandler) executeRule(rule engine.Rule, event *listener.Event, startTime time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ruleStartTime := time.Now()

	// 记录规则匹配
	metrics.RecordRuleMatched(rule.ID, event.Table, string(event.Action))

	logger.Infow("执行规则",
		"rule_id", rule.ID,
		"rule_name", rule.Name,
		"table", event.Table,
		"action", event.Action,
	)

	if err := h.executor.ExecuteActions(ctx, rule.Actions, event); err != nil {
		logger.Errorw("规则执行失败",
			"rule_id", rule.ID,
			"error", err,
			"table", event.Table,
			"action", event.Action,
		)

		// 记录失败的动作
		for _, action := range rule.Actions {
			metrics.RecordActionFailed(action.Type, rule.ID, "execution_error")
		}
	} else {
		logger.Infow("规则执行成功", "rule_id", rule.ID)

		// 记录成功的动作和处理耗时
		duration := time.Since(ruleStartTime).Seconds()
		for _, action := range rule.Actions {
			metrics.RecordActionExecuted(action.Type, rule.ID)
			metrics.RecordProcessingDuration(rule.ID, action.Type, duration)
		}
	}
}

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 创建应用
	app, err := NewApp(*configPath)
	if err != nil {
		// 如果日志未初始化，使用标准库
		if logger.Logger == nil {
			zap.L().Fatal("创建应用失败", zap.Error(err))
		} else {
			// 使用 fmt.Printf 打印错误，确保换行生效
			_, _ = fmt.Fprintf(os.Stderr, "创建应用失败: %v\n", err)
			// 如果错误包含详细堆栈信息，也打印出来
			if errVerbose, ok := err.(interface{ ErrorVerbose() string }); ok {
				_, _ = fmt.Fprintf(os.Stderr, "\n详细错误信息:\n%s\n", errVerbose.ErrorVerbose())
			}
			os.Exit(1)
		}
	}

	// 启动应用
	if err := app.Start(); err != nil {
		// 使用 fmt.Printf 打印错误，确保换行生效
		_, _ = fmt.Fprintf(os.Stderr, "启动应用失败: %v\n", err)
		// 如果错误包含详细堆栈信息，也打印出来
		if errVerbose, ok := err.(interface{ ErrorVerbose() string }); ok {
			_, _ = fmt.Fprintf(os.Stderr, "\n详细错误信息:\n%s\n", errVerbose.ErrorVerbose())
		}
		os.Exit(1)
	}
}
