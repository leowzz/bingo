package listener

import (
	"context"
	"fmt"
	"time"

	"bingo/internal/logger"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	"github.com/go-mysql-org/go-mysql/schema"
)

// EventHandler 事件处理器接口
type EventHandler interface {
	OnEvent(event *Event) error
}

// BinlogListener Binlog 监听器
type BinlogListener struct {
	canal        *canal.Canal
	handler      EventHandler
	filterTables map[string]bool
	posStore     PositionStore // 位置存储
}

// NewBinlogListener 创建新的 Binlog 监听器
func NewBinlogListener(cfg *canal.Config, handler EventHandler, tables []string) (*BinlogListener, error) {
	return NewBinlogListenerWithPositionStore(cfg, handler, tables, nil)
}

// NewBinlogListenerWithPositionStore 创建带位置存储的 Binlog 监听器
func NewBinlogListenerWithPositionStore(cfg *canal.Config, handler EventHandler, tables []string, posStore PositionStore) (*BinlogListener, error) {
	c, err := canal.NewCanal(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Canal 实例失败: %w", err)
	}

	filterTables := make(map[string]bool)
	for _, table := range tables {
		filterTables[table] = true
	}

	listener := &BinlogListener{
		canal:        c,
		handler:      handler,
		filterTables: filterTables,
		posStore:     posStore,
	}

	// 注册事件处理器
	c.SetEventHandler(listener)

	return listener, nil
}

// Start 启动监听
func (l *BinlogListener) Start() error {
	var pos mysql.Position
	var err error

	// 如果配置了位置存储，尝试从存储中加载位置
	if l.posStore != nil {
		ctx := context.Background()
		savedPos, err := l.posStore.Load(ctx)
		if err != nil {
			logger.Warnw("从位置存储加载失败，将使用当前位置", "error", err)
		} else if savedPos != nil {
			pos = *savedPos
			logger.Infow("从位置存储加载 Binlog 位置", "file", pos.Name, "position", pos.Pos)
			return l.canal.RunFrom(pos)
		}
	}

	// 如果没有保存的位置，获取 master 的 binlog 位置信息
	masterPos, err := l.canal.GetMasterPos()
	if err != nil {
		return fmt.Errorf("获取 Master Binlog 位置失败: %w", err)
	}

	// 从当前位置开始监听 binlog
	return l.canal.RunFrom(masterPos)
}

// StartFromPosition 从指定位置开始监听
func (l *BinlogListener) StartFromPosition(file string, position uint32) error {
	var pos mysql.Position
	if file != "" {
		pos.Name = file
		pos.Pos = position
	} else {
		// 如果未指定文件，获取当前位置
		masterPos, err := l.canal.GetMasterPos()
		if err != nil {
			return fmt.Errorf("获取 Master 位置失败: %w", err)
		}
		pos = masterPos
		if position > 0 {
			pos.Pos = position
		}
	}
	return l.canal.RunFrom(pos)
}

// Close 关闭监听器
func (l *BinlogListener) Close() {
	if l.canal != nil {
		l.canal.Close()
	}
}

// OnRow 实现 canal.EventHandler 接口
func (l *BinlogListener) OnRow(e *canal.RowsEvent) error {
	tableName := fmt.Sprintf("%s.%s", e.Table.Schema, e.Table.Name)

	// Debug: 打印收到的 Binlog 行事件
	logger.Debugw("收到 Binlog 行事件",
		"table", tableName,
		"action", e.Action,
		"rows_count", len(e.Rows),
	)

	// 如果配置了表过滤，只处理指定的表
	if len(l.filterTables) > 0 {
		if !l.filterTables[tableName] {
			logger.Debugw("表不在过滤列表中，跳过", "table", tableName)
			return nil
		}
	}

	// 获取列信息
	columns := e.Table.Columns

	// 处理不同类型的操作
	switch e.Action {
	case canal.InsertAction:
		return l.handleInsert(e, columns, tableName)
	case canal.DeleteAction:
		return l.handleDelete(e, columns, tableName)
	case canal.UpdateAction:
		return l.handleUpdate(e, columns, tableName)
	default:
		logger.Debugw("未知的操作类型", "action", e.Action)
		return nil
	}
}

// handleInsert 处理 INSERT 操作
func (l *BinlogListener) handleInsert(e *canal.RowsEvent, columns []schema.TableColumn, tableName string) error {
	for _, row := range e.Rows {
		newRow := rowToMap(columns, row)
		event := &Event{
			Table:     tableName,
			Action:    ActionInsert,
			Timestamp: time.Now(),
			NewRow:    newRow,
			Schema:    e.Table.Schema,
			TableName: e.Table.Name,
		}
		if err := l.handler.OnEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// handleDelete 处理 DELETE 操作
func (l *BinlogListener) handleDelete(e *canal.RowsEvent, columns []schema.TableColumn, tableName string) error {
	for _, row := range e.Rows {
		oldRow := rowToMap(columns, row)
		event := &Event{
			Table:     tableName,
			Action:    ActionDelete,
			Timestamp: time.Now(),
			OldRow:    oldRow,
			Schema:    e.Table.Schema,
			TableName: e.Table.Name,
		}
		if err := l.handler.OnEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// handleUpdate 处理 UPDATE 操作
func (l *BinlogListener) handleUpdate(e *canal.RowsEvent, columns []schema.TableColumn, tableName string) error {
	// UPDATE: 奇数索引是更新前的数据，偶数索引是更新后的数据
	for i := 0; i < len(e.Rows); i += 2 {
		if i+1 < len(e.Rows) {
			oldRow := rowToMap(columns, e.Rows[i])
			newRow := rowToMap(columns, e.Rows[i+1])
			event := &Event{
				Table:     tableName,
				Action:    ActionUpdate,
				Timestamp: time.Now(),
				OldRow:    oldRow,
				NewRow:    newRow,
				Schema:    e.Table.Schema,
				TableName: e.Table.Name,
			}
			if err := l.handler.OnEvent(event); err != nil {
				return err
			}
		}
	}
	return nil
}

// OnRotate 实现 canal.EventHandler 接口
func (l *BinlogListener) OnRotate(header *replication.EventHeader, rotateEvent *replication.RotateEvent) error {
	return nil
}

// OnTableChanged 实现 canal.EventHandler 接口
func (l *BinlogListener) OnTableChanged(header *replication.EventHeader, schema string, table string) error {
	return nil
}

// OnDDL 实现 canal.EventHandler 接口（DDL 操作暂不处理）
func (l *BinlogListener) OnDDL(header *replication.EventHeader, nextPos mysql.Position, queryEvent *replication.QueryEvent) error {
	// DDL 操作（CREATE, ALTER, DROP 等）暂不处理
	return nil
}

// OnXID 实现 canal.EventHandler 接口（事务提交事件）
func (l *BinlogListener) OnXID(header *replication.EventHeader, nextPos mysql.Position) error {
	// 事务提交事件，暂不处理
	return nil
}

// OnGTID 实现 canal.EventHandler 接口（GTID 事件）
func (l *BinlogListener) OnGTID(header *replication.EventHeader, gtidEvent mysql.BinlogGTIDEvent) error {
	// GTID 事件，暂不处理
	return nil
}

// OnPosSynced 实现 canal.EventHandler 接口（位置同步事件）
func (l *BinlogListener) OnPosSynced(header *replication.EventHeader, pos mysql.Position, set mysql.GTIDSet, force bool) error {
	// 如果配置了位置存储，保存位置
	if l.posStore != nil && force {
		ctx := context.Background()
		if err := l.posStore.Save(ctx, pos); err != nil {
			logger.Warnw("保存位置到存储失败", "error", err, "file", pos.Name, "position", pos.Pos)
		} else {
			logger.Debugw("已保存 Binlog 位置", "file", pos.Name, "position", pos.Pos)
		}
	}
	return nil
}

// OnRowsQueryEvent 实现 canal.EventHandler 接口
func (l *BinlogListener) OnRowsQueryEvent(e *replication.RowsQueryEvent) error {
	return nil
}

// String 实现 canal.EventHandler 接口
func (l *BinlogListener) String() string {
	return "BinlogListener"
}

// rowToMap 将行数据转换为 key-value 格式的 map
func rowToMap(columns []schema.TableColumn, row []interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for i, col := range columns {
		if i < len(row) {
			result[col.Name] = row[i]
		}
	}
	return result
}
