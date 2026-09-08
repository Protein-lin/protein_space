-- WAF Agent MySQL 8.0 schema
-- API Key 只允许保存应用层加密后的密文，不要把明文写入数据库。
CREATE DATABASE IF NOT EXISTS waf_agent
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE waf_agent;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL,
  email VARCHAR(191) NULL,
  password_hash VARCHAR(255) NOT NULL,
  status ENUM('active','disabled') NOT NULL DEFAULT 'active',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_users_username (username),
  UNIQUE KEY uk_users_email (email)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS user_sessions (
  id CHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_sessions_user (user_id),
  KEY idx_sessions_expiry (expires_at),
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS user_api_keys (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(100) NOT NULL,
  key_prefix VARCHAR(16) NOT NULL,
  key_hash CHAR(64) NOT NULL,
  last_used_at DATETIME(3) NULL,
  expires_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_api_key_hash (key_hash),
  KEY idx_user_api_keys_user (user_id),
  CONSTRAINT fk_user_api_keys_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS auth_ip_whitelist (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  cidr VARCHAR(64) NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT '',
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_auth_ip_whitelist_cidr (cidr),
  KEY idx_auth_ip_whitelist_enabled (enabled),
  CONSTRAINT fk_auth_ip_whitelist_creator FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB;

-- 仅示例：本机健康检查可放行；公网服务器不建议加入大网段。
INSERT IGNORE INTO auth_ip_whitelist (cidr, description) VALUES
  ('127.0.0.1/32', 'local IPv4'),
  ('::1/128', 'local IPv6');

-- 一套可复用的 OpenAI-compatible 服务配置。
-- api_key_ciphertext 由 API 使用 APP_ENCRYPTION_KEY 加密后写入。
CREATE TABLE IF NOT EXISTS provider_configs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(100) NOT NULL,
  base_url VARCHAR(500) NOT NULL,
  api_key_ciphertext TEXT NOT NULL,
  api_key_hint VARCHAR(16) NULL,
  default_model VARCHAR(100) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_provider_configs_creator (created_by),
  CONSTRAINT fk_provider_configs_creator FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB;

-- 用户可使用哪些配置；可通过 role 控制 owner/member/read_only。
CREATE TABLE IF NOT EXISTS user_provider_configs (
  user_id BIGINT UNSIGNED NOT NULL,
  provider_config_id BIGINT UNSIGNED NOT NULL,
  role ENUM('owner','member','read_only') NOT NULL DEFAULT 'member',
  is_default TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id, provider_config_id),
  KEY idx_upc_config (provider_config_id),
  CONSTRAINT fk_upc_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_upc_config FOREIGN KEY (provider_config_id) REFERENCES provider_configs(id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS conversations (
  id CHAR(36) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  provider_config_id BIGINT UNSIGNED NULL,
  title VARCHAR(255) NOT NULL DEFAULT '新对话',
  model VARCHAR(100) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_conversations_user_updated (user_id, updated_at),
  CONSTRAINT fk_conversations_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_conversations_provider FOREIGN KEY (provider_config_id) REFERENCES provider_configs(id) ON DELETE SET NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS messages (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  conversation_id CHAR(36) NOT NULL,
  role ENUM('system','user','assistant','tool') NOT NULL,
  content LONGTEXT NOT NULL,
  request_id VARCHAR(128) NULL,
  sequence_no INT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_messages_sequence (conversation_id, sequence_no),
  KEY idx_messages_conversation_created (conversation_id, created_at),
  CONSTRAINT fk_messages_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS usage_records (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  conversation_id CHAR(36) NULL,
  provider_config_id BIGINT UNSIGNED NULL,
  model VARCHAR(100) NOT NULL,
  prompt_tokens INT UNSIGNED NOT NULL DEFAULT 0,
  completion_tokens INT UNSIGNED NOT NULL DEFAULT 0,
  total_tokens INT UNSIGNED NOT NULL DEFAULT 0,
  latency_ms INT UNSIGNED NULL,
  status ENUM('success','error') NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_usage_user_created (user_id, created_at),
  CONSTRAINT fk_usage_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_usage_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE SET NULL,
  CONSTRAINT fk_usage_provider FOREIGN KEY (provider_config_id) REFERENCES provider_configs(id) ON DELETE SET NULL
) ENGINE=InnoDB;

-- 本地开发管理员账号示例：生产环境必须替换 password_hash。
-- INSERT INTO users (username, email, password_hash) VALUES ('admin', 'admin@example.com', '<bcrypt-hash>');
