package config

import (
	"os"
	"testing"
)

func TestConnCfgHash(t *testing.T) {
	cfg := MySQLConfig{
		Host:     "192.168.177.100",
		Port:     3306,
		User:     "root",
		Password: "test_password",
		Database: "test",
	}

	hash := cfg.ConnCfgHash()
	if hash == "" {
		t.Error("hash 值不应为空")
	}
	if len(hash) != 12 {
		t.Errorf("hash 长度应为 12，实际为 %d", len(hash))
	}

	// 相同配置应该产生相同的 hash
	hash2 := cfg.ConnCfgHash()
	if hash != hash2 {
		t.Error("相同配置应产生相同的 hash")
	}

	// 不同配置应该产生不同的 hash
	cfg2 := MySQLConfig{
		Host:     "192.168.177.101",
		Port:     3306,
		User:     "root",
		Password: "test_password",
		Database: "test",
	}
	hash3 := cfg2.ConnCfgHash()
	if hash == hash3 {
		t.Error("不同配置应产生不同的 hash")
	}
}

func TestResolveRedisStoreKeyTemplate(t *testing.T) {
	cfg := &Config{
		MySQL: MySQLConfig{
			Host:     "192.168.177.100",
			Port:     3306,
			User:     "root",
			Password: "test_password",
			Database: "test",
		},
		Binlog: BinlogConfig{
			RedisStoreKey: "bingo:binlog:position:{{ .ConnCfgHash }}",
		},
	}

	err := cfg.resolveRedisStoreKeyTemplate()
	if err != nil {
		t.Fatalf("解析模板失败: %v", err)
	}

	// 验证模板已被替换
	if cfg.Binlog.RedisStoreKey == "bingo:binlog:position:{{ .ConnCfgHash }}" {
		t.Error("模板未被替换")
	}

	// 验证结果格式
	expectedPrefix := "bingo:binlog:position:"
	if len(cfg.Binlog.RedisStoreKey) <= len(expectedPrefix) {
		t.Errorf("解析后的键名长度不足: %s", cfg.Binlog.RedisStoreKey)
	}
	if cfg.Binlog.RedisStoreKey[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("解析后的键名前缀不正确: %s", cfg.Binlog.RedisStoreKey)
	}

	// 验证 hash 部分存在
	hashPart := cfg.Binlog.RedisStoreKey[len(expectedPrefix):]
	if len(hashPart) != 12 {
		t.Errorf("hash 部分长度应为 12，实际为 %d: %s", len(hashPart), hashPart)
	}
}

func TestResolveRedisStoreKeyTemplate_NoTemplate(t *testing.T) {
	cfg := &Config{
		MySQL: MySQLConfig{
			Host:     "192.168.177.100",
			Port:     3306,
			User:     "root",
			Password: "test_password",
			Database: "test",
		},
		Binlog: BinlogConfig{
			RedisStoreKey: "bingo:binlog:position",
		},
	}

	originalKey := cfg.Binlog.RedisStoreKey
	err := cfg.resolveRedisStoreKeyTemplate()
	if err != nil {
		t.Fatalf("解析模板失败: %v", err)
	}

	// 验证没有模板时，键名保持不变
	if cfg.Binlog.RedisStoreKey != originalKey {
		t.Errorf("无模板时键名不应改变，期望: %s, 实际: %s", originalKey, cfg.Binlog.RedisStoreKey)
	}
}

func TestLoadConfig_WithTemplate(t *testing.T) {
	// 创建临时配置文件
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `mysql:
  host: "192.168.177.100"
  port: 3306
  user: "root"
  password: "test_password"
  database: "test"
binlog:
  redis_store_key: "bingo:binlog:position:{{ .ConnCfgHash }}"
`
	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("写入配置文件失败: %v", err)
	}
	tmpFile.Close()

	// 加载配置
	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证模板已被解析
	if cfg.Binlog.RedisStoreKey == "bingo:binlog:position:{{ .ConnCfgHash }}" {
		t.Error("模板未被解析")
	}

	expectedPrefix := "bingo:binlog:position:"
	if len(cfg.Binlog.RedisStoreKey) <= len(expectedPrefix) {
		t.Errorf("解析后的键名长度不足: %s", cfg.Binlog.RedisStoreKey)
	}
	if cfg.Binlog.RedisStoreKey[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("解析后的键名前缀不正确: %s", cfg.Binlog.RedisStoreKey)
	}

	// 验证 hash 部分存在
	hashPart := cfg.Binlog.RedisStoreKey[len(expectedPrefix):]
	if len(hashPart) != 12 {
		t.Errorf("hash 部分长度应为 12，实际为 %d: %s", len(hashPart), hashPart)
	}
}
