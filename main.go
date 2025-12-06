package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bingo/config"
	"bingo/engine"
	"bingo/executor"
	"bingo/internal/logger"
	"bingo/internal/metrics"
	"bingo/listener"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// App 应用主结构
type App struct {
	config   *config.Config
	matcher  *engine.Matcher
	executor *executor.Executor
	listener *listener.BinlogListener
}

// NewApp 创建新的应用实例
func NewApp(cfgPath string) (*App, error) {
	// 加载配置
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}

	// 初始化日志
	if err := logger.InitLogger(
		cfg.Logging.Level,
		cfg.Logging.Format,
		cfg.Logging.Output,
		cfg.Logging.File,
		cfg.Logging.MaxSize,
		cfg.Logging.MaxBackups,
		cfg.Logging.MaxAge,
	); err != nil {
		return nil, err
	}
	defer logger.Sync()

	// 加载规则和 Redis 连接配置
	rulesConfig, err := engine.LoadRulesWithRedisConnections(cfg.RulesFile)
	if err != nil {
		return nil, err
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
			if err := redisExec.AddConnection(conn.Name, conn.Addr, conn.Password, conn.DB); err != nil {
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

	// 创建事件处理器
	eventHandler := &EventHandler{
		matcher:  matcher,
		executor: exec,
	}

	// 创建位置存储（如果配置了系统 Redis 且启用了位置存储）
	var posStore listener.PositionStore
	if cfg.Binlog.UseRedisStore && systemRedisClient != nil {
		posStore = listener.NewRedisPositionStore(systemRedisClient, cfg.Binlog.RedisStoreKey)
		logger.Infof("已启用 Redis 位置存储，键名: %s", cfg.Binlog.RedisStoreKey)
	}

	// 创建监听器（暂时不指定表，监控所有表）
	// 配置位置保存间隔和是否在事务提交时保存
	saveInterval := time.Duration(cfg.Binlog.SaveInterval) * time.Second
	if saveInterval == 0 {
		saveInterval = 5 * time.Second // 默认 5 秒
	}
	saveOnTransaction := cfg.Binlog.SaveOnTransaction
	binlogListener, err := listener.NewBinlogListenerWithPositionStoreAndConfig(
		canalCfg,
		eventHandler,
		nil,
		posStore,
		saveInterval,
		saveOnTransaction,
	)
	if err != nil {
		return nil, err
	}

	return &App{
		config:   cfg,
		matcher:  matcher,
		executor: exec,
		listener: binlogListener,
	}, nil
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

	// 启动监听
	if a.config.Binlog.File != "" {
		logger.Infof("从 Binlog 位置: %s:%d 开始监听", a.config.Binlog.File, a.config.Binlog.Position)
		return a.listener.StartFromPosition(a.config.Binlog.File, a.config.Binlog.Position)
	}

	logger.Info("从当前位置开始监听 Binlog 变更...")
	return a.listener.Start()
}

// Stop 停止应用
func (a *App) Stop() {
	if a.listener != nil {
		a.listener.Close()
	}
	logger.Info("服务已停止")
	logger.Sync()
}

// EventHandler 事件处理器
type EventHandler struct {
	matcher  *engine.Matcher
	executor *executor.Executor
}

// OnEvent 处理事件
func (h *EventHandler) OnEvent(event *listener.Event) error {
	startTime := time.Now()

	// 记录事件
	metrics.RecordEvent(event.Table, string(event.Action))

	// Debug: 打印收到的事件信息
	logger.Debugw("收到事件",
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
		return err
	}

	if len(matchedRules) == 0 {
		// 没有匹配的规则，直接返回
		logger.Debugw("未匹配到规则", "table", event.Table, "action", event.Action)
		return nil
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

	// 执行匹配规则的动作
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, rule := range matchedRules {
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

	// 记录总处理耗时
	totalDuration := time.Since(startTime).Seconds()
	metrics.RecordProcessingDuration("total", "event", totalDuration)

	return nil
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
			logger.Fatalw("创建应用失败", "error", err)
		}
	}

	// 启动应用
	if err := app.Start(); err != nil {
		logger.Fatalw("启动应用失败", "error", err)
	}
}
