#!/usr/bin/env python3
"""
数据库操作工具脚本

从 config.yaml 读取数据库配置，循环执行各种数据库操作。
支持 MySQL 和 Redis 数据库。
"""

import time
from pathlib import Path
from typing import Optional
from contextlib import contextmanager

import yaml
import pymysql
import redis
from pydantic import BaseModel, Field
from loguru import logger

BASE_DIR = Path(__file__).parent.parent

config_file = BASE_DIR / "config.yaml"

logger.info(f"{BASE_DIR=}. {config_file=}")
# ============================================================================
# Pydantic 配置模型
# ============================================================================

class MySQLConfig(BaseModel):
    """MySQL 连接配置"""
    host: str = Field(..., description="MySQL 服务器地址")
    port: int = Field(default=3306, description="MySQL 端口")
    user: str = Field(..., description="MySQL 用户名")
    password: str = Field(..., description="MySQL 密码")
    database: Optional[str] = Field(default=None, description="数据库名")


class RedisConfig(BaseModel):
    """Redis 连接配置"""
    addr: str = Field(..., description="Redis 服务器地址")
    password: str = Field(default="", description="Redis 密码")
    db: int = Field(default=0, description="Redis 数据库编号")


class Config(BaseModel):
    """应用配置"""
    mysql: MySQLConfig
    system_redis: RedisConfig


# ============================================================================
# 数据库操作类
# ============================================================================

class MySQLOperator:
    """MySQL 数据库操作类"""

    def __init__(self, config: MySQLConfig):
        """
        初始化 MySQL 操作器

        :param config: MySQL 配置
        """
        self.config = config
        self.connection: Optional[pymysql.Connection] = None

    def connect(self) -> bool:
        """
        连接 MySQL 数据库

        :return: 连接是否成功
        """
        try:
            self.connection = pymysql.connect(
                host=self.config.host,
                port=self.config.port,
                user=self.config.user,
                password=self.config.password,
                database=self.config.database,
                charset='utf8mb4',
                cursorclass=pymysql.cursors.DictCursor,
                autocommit=True
            )
            logger.info(f"MySQL 连接成功: {self.config.host}:{self.config.port}")
            return True
        except Exception as e:
            logger.error(f"MySQL 连接失败: {e}")
            return False

    def close(self):
        """关闭数据库连接"""
        if self.connection:
            self.connection.close()
            logger.info("MySQL 连接已关闭")

    @contextmanager
    def get_cursor(self):
        """获取数据库游标的上下文管理器"""
        if not self.connection:
            raise RuntimeError("数据库未连接")
        cursor = self.connection.cursor()
        try:
            yield cursor
        finally:
            cursor.close()

    def execute_query(self, sql: str, params: Optional[tuple] = None) -> list:
        """
        执行查询语句

        :param sql: SQL 查询语句
        :param params: 查询参数
        :return: 查询结果列表
        """
        try:
            with self.get_cursor() as cursor:
                cursor.execute(sql, params)
                result = cursor.fetchall()
                logger.info(f"查询成功，返回 {len(result)} 条记录")
                return result
        except Exception as e:
            logger.error(f"查询执行失败: {e}")
            return []

    def execute_update(self, sql: str, params: Optional[tuple] = None) -> int:
        """
        执行更新语句（INSERT, UPDATE, DELETE）

        :param sql: SQL 更新语句
        :param params: 更新参数
        :return: 受影响的行数
        """
        try:
            with self.get_cursor() as cursor:
                affected_rows = cursor.execute(sql, params)
                logger.info(f"更新成功，影响 {affected_rows} 行")
                return affected_rows
        except Exception as e:
            logger.error(f"更新执行失败: {e}")
            return 0

    def show_databases(self) -> list:
        """显示所有数据库"""
        return self.execute_query("SHOW DATABASES")

    def show_tables(self) -> list:
        """显示当前数据库的所有表"""
        if not self.config.database:
            logger.warning("未指定数据库，无法显示表")
            return []
        return self.execute_query("SHOW TABLES")

    def get_table_info(self, table_name: str) -> list:
        """
        获取表结构信息

        :param table_name: 表名
        :return: 表结构信息
        """
        return self.execute_query(f"DESCRIBE {table_name}")

    def select_all(self, table_name: str, limit: int = 10) -> list:
        """
        查询表中的所有数据（限制条数）

        :param table_name: 表名
        :param limit: 限制返回条数
        :return: 查询结果
        """
        sql = f"SELECT * FROM {table_name} LIMIT %s"
        return self.execute_query(sql, (limit,))

    def insert_test_data(self, table_name: str, data: dict) -> int:
        """
        插入测试数据

        :param table_name: 表名
        :param data: 要插入的数据字典
        :return: 受影响的行数
        """
        columns = ', '.join(data.keys())
        placeholders = ', '.join(['%s'] * len(data))
        sql = f"INSERT INTO {table_name} ({columns}) VALUES ({placeholders})"
        return self.execute_update(sql, tuple(data.values()))

    def update_data(self, table_name: str, set_data: dict, where_condition: str, where_params: Optional[tuple] = None) -> int:
        """
        更新数据

        :param table_name: 表名
        :param set_data: 要更新的字段和值
        :param where_condition: WHERE 条件
        :param where_params: WHERE 条件参数
        :return: 受影响的行数
        """
        set_clause = ', '.join([f"{k} = %s" for k in set_data.keys()])
        sql = f"UPDATE {table_name} SET {set_clause} WHERE {where_condition}"
        params = tuple(set_data.values())
        if where_params:
            params += where_params
        return self.execute_update(sql, params)

    def delete_data(self, table_name: str, where_condition: str, where_params: Optional[tuple] = None) -> int:
        """
        删除数据

        :param table_name: 表名
        :param where_condition: WHERE 条件
        :param where_params: WHERE 条件参数
        :return: 受影响的行数
        """
        sql = f"DELETE FROM {table_name} WHERE {where_condition}"
        return self.execute_update(sql, where_params)

    def execute_ddl(self, sql: str) -> bool:
        """
        执行 DDL 语句（CREATE, ALTER, DROP）

        :param sql: DDL 语句
        :return: 是否成功
        """
        try:
            with self.get_cursor() as cursor:
                cursor.execute(sql)
                logger.info(f"DDL 执行成功: {sql[:50]}...")
                return True
        except Exception as e:
            logger.error(f"DDL 执行失败: {e}")
            return False

    def drop_table_if_exists(self, table_name: str) -> bool:
        """
        如果表存在则删除表

        :param table_name: 表名
        :return: 是否成功
        """
        sql = f"DROP TABLE IF EXISTS {table_name}"
        return self.execute_ddl(sql)

    def create_table(self, table_name: str, columns: dict) -> bool:
        """
        创建表

        :param table_name: 表名
        :param columns: 字段定义字典，格式: {"column_name": "column_definition"}
        :return: 是否成功
        """
        column_definitions = ', '.join([f"{name} {definition}" for name, definition in columns.items()])
        sql = f"CREATE TABLE IF NOT EXISTS {table_name} ({column_definitions}) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
        return self.execute_ddl(sql)

    def add_column(self, table_name: str, column_name: str, column_definition: str, after_column: Optional[str] = None) -> bool:
        """
        添加字段

        :param table_name: 表名
        :param column_name: 字段名
        :param column_definition: 字段定义（如 "VARCHAR(100) NOT NULL"）
        :param after_column: 在哪个字段之后添加（可选）
        :return: 是否成功
        """
        sql = f"ALTER TABLE {table_name} ADD COLUMN {column_name} {column_definition}"
        if after_column:
            sql += f" AFTER {after_column}"
        return self.execute_ddl(sql)

    def drop_column(self, table_name: str, column_name: str) -> bool:
        """
        删除字段

        :param table_name: 表名
        :param column_name: 字段名
        :return: 是否成功
        """
        sql = f"ALTER TABLE {table_name} DROP COLUMN {column_name}"
        return self.execute_ddl(sql)

    def count_rows(self, table_name: str) -> int:
        """
        统计表中的记录数

        :param table_name: 表名
        :return: 记录数
        """
        result = self.execute_query(f"SELECT COUNT(*) as count FROM {table_name}")
        if result:
            return result[0].get('count', 0)
        return 0


class RedisOperator:
    """Redis 数据库操作类"""

    def __init__(self, config: RedisConfig):
        """
        初始化 Redis 操作器

        :param config: Redis 配置
        """
        self.config = config
        self.client: Optional[redis.Redis] = None

    def connect(self) -> bool:
        """
        连接 Redis 数据库

        :return: 连接是否成功
        """
        try:
            # 解析地址（格式：host:port）
            if ':' in self.config.addr:
                host, port = self.config.addr.split(':', 1)
                port = int(port)
            else:
                host = self.config.addr
                port = 6379

            self.client = redis.Redis(
                host=host,
                port=port,
                password=self.config.password if self.config.password else None,
                db=self.config.db,
                decode_responses=True
            )
            # 测试连接
            self.client.ping()
            logger.info(f"Redis 连接成功: {host}:{port} (db={self.config.db})")
            return True
        except Exception as e:
            logger.error(f"Redis 连接失败: {e}")
            return False

    def close(self):
        """关闭 Redis 连接"""
        if self.client:
            self.client.close()
            logger.info("Redis 连接已关闭")

    def set_key(self, key: str, value: str, ex: Optional[int] = None) -> bool:
        """
        设置键值对

        :param key: 键名
        :param value: 值
        :param ex: 过期时间（秒）
        :return: 是否成功
        """
        try:
            result = self.client.set(key, value, ex=ex)
            logger.info(f"设置键成功: {key} = {value}")
            return result
        except Exception as e:
            logger.error(f"设置键失败: {e}")
            return False

    def get_key(self, key: str) -> Optional[str]:
        """
        获取键值

        :param key: 键名
        :return: 键值
        """
        try:
            value = self.client.get(key)
            if value:
                logger.info(f"获取键成功: {key} = {value}")
            else:
                logger.warning(f"键不存在: {key}")
            return value
        except Exception as e:
            logger.error(f"获取键失败: {e}")
            return None

    def delete_key(self, key: str) -> int:
        """
        删除键

        :param key: 键名
        :return: 删除的键数量
        """
        try:
            result = self.client.delete(key)
            logger.info(f"删除键成功: {key}，删除数量: {result}")
            return result
        except Exception as e:
            logger.error(f"删除键失败: {e}")
            return 0

    def list_keys(self, pattern: str = "*") -> list:
        """
        列出所有匹配的键

        :param pattern: 匹配模式
        :return: 键列表
        """
        try:
            keys = self.client.keys(pattern)
            logger.info(f"列出键成功，匹配 {len(keys)} 个键（模式: {pattern}）")
            return keys
        except Exception as e:
            logger.error(f"列出键失败: {e}")
            return []

    def hash_set(self, name: str, key: str, value: str) -> int:
        """
        设置哈希字段

        :param name: 哈希表名
        :param key: 字段名
        :param value: 字段值
        :return: 是否成功
        """
        try:
            result = self.client.hset(name, key, value)
            logger.info(f"设置哈希字段成功: {name}.{key} = {value}")
            return result
        except Exception as e:
            logger.error(f"设置哈希字段失败: {e}")
            return 0

    def hash_get(self, name: str, key: str) -> Optional[str]:
        """
        获取哈希字段

        :param name: 哈希表名
        :param key: 字段名
        :return: 字段值
        """
        try:
            value = self.client.hget(name, key)
            if value:
                logger.info(f"获取哈希字段成功: {name}.{key} = {value}")
            else:
                logger.warning(f"哈希字段不存在: {name}.{key}")
            return value
        except Exception as e:
            logger.error(f"获取哈希字段失败: {e}")
            return None

    def list_push(self, name: str, *values: str) -> int:
        """
        向列表添加元素

        :param name: 列表名
        :param values: 要添加的值
        :return: 列表长度
        """
        try:
            result = self.client.lpush(name, *values)
            logger.info(f"向列表添加元素成功: {name}，添加 {len(values)} 个元素")
            return result
        except Exception as e:
            logger.error(f"向列表添加元素失败: {e}")
            return 0

    def list_get(self, name: str, start: int = 0, end: int = -1) -> list:
        """
        获取列表元素

        :param name: 列表名
        :param start: 起始索引
        :param end: 结束索引
        :return: 元素列表
        """
        try:
            result = self.client.lrange(name, start, end)
            logger.info(f"获取列表元素成功: {name}，返回 {len(result)} 个元素")
            return result
        except Exception as e:
            logger.error(f"获取列表元素失败: {e}")
            return []


# ============================================================================
# 主程序
# ============================================================================

def load_config(config_path: str) -> Optional[Config]:
    """
    加载配置文件

    :param config_path: 配置文件路径
    :return: 配置对象
    """
    try:
        config_file = Path(config_path)
        if not config_file.exists():
            logger.error(f"配置文件不存在: {config_path}")
            return None

        with open(config_file, 'r', encoding='utf-8') as f:
            config_data = yaml.safe_load(f)

        config = Config(**config_data)
        logger.info(f"配置文件加载成功: {config_path}")
        return config
    except Exception as e:
        logger.error(f"加载配置文件失败: {e}")
        return None


def run_ddl_dml_operations(mysql_op: MySQLOperator):
    """
    执行完整的 DDL/DML 操作流程

    :param mysql_op: MySQL 操作器
    """
    table_name = "users_001"
    
    logger.info(f"\n{'='*60}")
    logger.info("开始执行 DDL/DML 操作流程")
    logger.info(f"{'='*60}")

    # 步骤 1: 删除表（如果存在）
    logger.info(f"\n[步骤 1] 删除表 {table_name}（如果存在）")
    mysql_op.drop_table_if_exists(table_name)
    time.sleep(1)

    # 步骤 2: 创建表 users_001，带初始简单字段
    logger.info(f"\n[步骤 2] 创建表 {table_name}，带初始简单字段")
    initial_columns = {
        "id": "INT AUTO_INCREMENT PRIMARY KEY",
        "username": "VARCHAR(50) NOT NULL",
        "email": "VARCHAR(100)",
        "created_at": "TIMESTAMP DEFAULT CURRENT_TIMESTAMP"
    }
    mysql_op.create_table(table_name, initial_columns)
    
    # 查看表结构
    logger.info(f"\n查看表 {table_name} 的结构:")
    table_info = mysql_op.get_table_info(table_name)
    for col in table_info:
        logger.info(f"  - {col.get('Field')}: {col.get('Type')} {col.get('Null')} {col.get('Key')}")
    time.sleep(1)

    # 步骤 3: 向表中插入初始数据
    logger.info(f"\n[步骤 3] 向表 {table_name} 插入初始数据")
    initial_data = [
        {"username": "user1", "email": "user1@example.com"},
        {"username": "user2", "email": "user2@example.com"},
        {"username": "user3", "email": "user3@example.com"},
        {"username": "user4", "email": "user4@example.com"},
        {"username": "user5", "email": "user5@example.com"},
    ]
    for data in initial_data:
        mysql_op.insert_test_data(table_name, data)
    
    # 查看插入后的数据
    logger.info(f"\n查看插入后的数据（前5条）:")
    rows = mysql_op.select_all(table_name, limit=5)
    for row in rows:
        logger.info(f"  {row}")
    logger.info(f"当前表中共有 {mysql_op.count_rows(table_name)} 条记录")
    time.sleep(1)

    # 步骤 4: 删除部分数据
    logger.info(f"\n[步骤 4] 删除部分数据（删除 username='user1' 的记录）")
    deleted_count = mysql_op.delete_data(table_name, "username = %s", ("user1",))
    logger.info(f"删除了 {deleted_count} 条记录")
    logger.info(f"删除后表中共有 {mysql_op.count_rows(table_name)} 条记录")
    time.sleep(1)

    # 步骤 5: 增加字段
    logger.info(f"\n[步骤 5] 增加字段 phone 和 age")
    mysql_op.add_column(table_name, "phone", "VARCHAR(20)", after_column="email")
    mysql_op.add_column(table_name, "age", "INT", after_column="phone")
    
    # 查看更新后的表结构
    logger.info(f"\n查看更新后的表结构:")
    table_info = mysql_op.get_table_info(table_name)
    for col in table_info:
        logger.info(f"  - {col.get('Field')}: {col.get('Type')} {col.get('Null')} {col.get('Key')}")
    time.sleep(1)

    # 步骤 6: 插入新数据（包含新字段）
    logger.info(f"\n[步骤 6] 插入新数据（包含新字段 phone 和 age）")
    new_data = [
        {"username": "user6", "email": "user6@example.com", "phone": "13800138006", "age": 25},
        {"username": "user7", "email": "user7@example.com", "phone": "13800138007", "age": 30},
        {"username": "user8", "email": "user8@example.com", "phone": "13800138008", "age": 28},
    ]
    for data in new_data:
        mysql_op.insert_test_data(table_name, data)
    
    # 查看插入后的数据
    logger.info(f"\n查看插入后的数据（前5条）:")
    rows = mysql_op.select_all(table_name, limit=5)
    for row in rows:
        logger.info(f"  {row}")
    logger.info(f"当前表中共有 {mysql_op.count_rows(table_name)} 条记录")
    time.sleep(1)

    # 步骤 7: 再次删除部分数据
    logger.info(f"\n[步骤 7] 再次删除部分数据（删除 age < 30 的记录）")
    deleted_count = mysql_op.delete_data(table_name, "age < %s", (30,))
    logger.info(f"删除了 {deleted_count} 条记录")
    logger.info(f"删除后表中共有 {mysql_op.count_rows(table_name)} 条记录")
    time.sleep(1)

    # 步骤 8: 删除字段
    logger.info(f"\n[步骤 8] 删除字段 phone")
    mysql_op.drop_column(table_name, "phone")
    
    # 查看更新后的表结构
    logger.info(f"\n查看更新后的表结构:")
    table_info = mysql_op.get_table_info(table_name)
    for col in table_info:
        logger.info(f"  - {col.get('Field')}: {col.get('Type')} {col.get('Null')} {col.get('Key')}")
    time.sleep(1)

    # 步骤 9: 插入数据（不包含已删除的字段）
    logger.info(f"\n[步骤 9] 插入数据（不包含已删除的 phone 字段）")
    final_data = [
        {"username": "user9", "email": "user9@example.com", "age": 35},
        {"username": "user10", "email": "user10@example.com", "age": 40},
    ]
    for data in final_data:
        mysql_op.insert_test_data(table_name, data)
    
    # 查看最终数据
    logger.info(f"\n查看最终数据（所有记录）:")
    rows = mysql_op.select_all(table_name, limit=100)
    for row in rows:
        logger.info(f"  {row}")
    logger.info(f"最终表中共有 {mysql_op.count_rows(table_name)} 条记录")

    logger.info(f"\n{'='*60}")
    logger.info("DDL/DML 操作流程完成！")
    logger.info(f"{'='*60}\n")


def run_mysql_operations(mysql_op: MySQLOperator, iterations: int = 5):
    """
    循环执行 MySQL 操作

    :param mysql_op: MySQL 操作器
    :param iterations: 循环次数
    """
    logger.info(f"开始执行 MySQL 操作，共 {iterations} 次循环")

    for i in range(iterations):
        logger.info(f"\n{'='*60}")
        logger.info(f"MySQL 操作循环 #{i+1}/{iterations}")
        logger.info(f"{'='*60}")

        # 1. 显示数据库列表
        logger.info("\n[操作 1] 显示数据库列表")
        databases = mysql_op.show_databases()
        for db in databases[:5]:  # 只显示前5个
            logger.info(f"  - {list(db.values())[0]}")

        # 2. 如果指定了数据库，显示表列表
        if mysql_op.config.database:
            logger.info(f"\n[操作 2] 显示数据库 '{mysql_op.config.database}' 的表列表")
            tables = mysql_op.show_tables()
            for table in tables[:5]:  # 只显示前5个
                logger.info(f"  - {list(table.values())[0]}")

            # 3. 如果有表，查询表数据
            if tables:
                table_name = list(tables[0].values())[0]
                logger.info(f"\n[操作 3] 查询表 '{table_name}' 的数据（限制10条）")
                rows = mysql_op.select_all(table_name, limit=10)
                for row in rows[:3]:  # 只显示前3条
                    logger.info(f"  {row}")

        # 4. 执行一个简单的查询
        logger.info("\n[操作 4] 执行系统查询")
        result = mysql_op.execute_query("SELECT VERSION() as version, NOW() as current_time")
        if result:
            logger.info(f"  MySQL 版本: {result[0].get('version')}")
            logger.info(f"  当前时间: {result[0].get('current_time')}")

        # 等待一段时间
        if i < iterations - 1:
            logger.info(f"\n等待 2 秒后继续下一次循环...")
            time.sleep(2)


def run_redis_operations(redis_op: RedisOperator, iterations: int = 5):
    """
    循环执行 Redis 操作

    :param redis_op: Redis 操作器
    :param iterations: 循环次数
    """
    logger.info(f"开始执行 Redis 操作，共 {iterations} 次循环")

    for i in range(iterations):
        logger.info(f"\n{'='*60}")
        logger.info(f"Redis 操作循环 #{i+1}/{iterations}")
        logger.info(f"{'='*60}")

        # 1. 设置键值对
        test_key = f"test:key:{i+1}"
        test_value = f"test_value_{i+1}_{int(time.time())}"
        logger.info(f"\n[操作 1] 设置键值对")
        redis_op.set_key(test_key, test_value, ex=60)

        # 2. 获取键值
        logger.info(f"\n[操作 2] 获取键值")
        value = redis_op.get_key(test_key)
        if value:
            logger.info(f"  获取到的值: {value}")

        # 3. 哈希操作
        hash_name = "test:hash"
        hash_key = f"field_{i+1}"
        hash_value = f"value_{i+1}"
        logger.info(f"\n[操作 3] 设置哈希字段")
        redis_op.hash_set(hash_name, hash_key, hash_value)

        logger.info(f"\n[操作 4] 获取哈希字段")
        hash_val = redis_op.hash_get(hash_name, hash_key)
        if hash_val:
            logger.info(f"  获取到的哈希值: {hash_val}")

        # 4. 列表操作
        list_name = "test:list"
        logger.info(f"\n[操作 5] 向列表添加元素")
        redis_op.list_push(list_name, f"item_{i+1}", f"data_{i+1}")

        logger.info(f"\n[操作 6] 获取列表元素")
        list_items = redis_op.list_get(list_name, 0, 4)
        for item in list_items:
            logger.info(f"  - {item}")

        # 5. 列出所有测试键
        logger.info(f"\n[操作 7] 列出所有测试键")
        keys = redis_op.list_keys("test:*")
        logger.info(f"  找到 {len(keys)} 个测试键")

        # 等待一段时间
        if i < iterations - 1:
            logger.info(f"\n等待 2 秒后继续下一次循环...")
            time.sleep(2)


def main():
    """主函数"""
    # 使用已定义的配置文件路径
    config_path = str(config_file)

    # 加载配置
    config = load_config(config_path)
    if not config:
        logger.error("无法加载配置，程序退出")
        return

    # 初始化 MySQL 操作器
    mysql_op = MySQLOperator(config.mysql)
    if not mysql_op.connect():
        logger.error("MySQL 连接失败，跳过 MySQL 操作")
        return
    else:
        try:
            # 执行 DDL/DML 操作流程
            run_ddl_dml_operations(mysql_op)
            
            # 执行常规 MySQL 操作（可选）
            # run_mysql_operations(mysql_op, iterations=5)
        finally:
            mysql_op.close()

    # # 初始化 Redis 操作器
    # redis_op = RedisOperator(config.system_redis)
    # if not redis_op.connect():
    #     logger.error("Redis 连接失败，跳过 Redis 操作")
    # else:
    #     try:
    #         # 执行 Redis 操作
    #         run_redis_operations(redis_op, iterations=5)
    #     finally:
    #         redis_op.close()

    # logger.info("\n所有操作完成！")


if __name__ == "__main__":
    main()

