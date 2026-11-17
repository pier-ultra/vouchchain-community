-- 产品② 治理底座：SCIM 2.0 账号同步（RFC 7643 / 7644）
-- 身份提供方（Casdoor / 飞书 / AD）把人员与组推过来，本系统据此自动开户、改角色、离职禁用。
-- 全部为开源社区版能力，无企业版分层、无 license 开关。幂等可重复执行。

-- 由 SCIM 同步过来的用户。保留原始文档便于排查 IdP 字段差异；active=false 即离职/禁用。
CREATE TABLE IF NOT EXISTS scim_user (
  id BIGSERIAL PRIMARY KEY,
  external_id VARCHAR(200) NOT NULL DEFAULT '',
  username VARCHAR(100) NOT NULL,
  display_name VARCHAR(200) NOT NULL DEFAULT '',
  email VARCHAR(200) NOT NULL DEFAULT '',
  title VARCHAR(200) NOT NULL DEFAULT '',
  department VARCHAR(200) NOT NULL DEFAULT '',
  active BOOLEAN NOT NULL DEFAULT TRUE,
  groups TEXT NOT NULL DEFAULT '[]',        -- JSON 数组：所属组的 display / id
  roles TEXT NOT NULL DEFAULT '[]',         -- JSON 数组：同步后实际生效的角色编码
  raw TEXT NOT NULL DEFAULT '{}',           -- 最近一次 SCIM User 文档
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  last_synced_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_scim_user_username ON scim_user(username);
CREATE UNIQUE INDEX IF NOT EXISTS uk_scim_user_external ON scim_user(external_id) WHERE external_id <> '';
CREATE INDEX IF NOT EXISTS idx_scim_user_active ON scim_user(active);

-- SCIM Group：组成员变化时重算所属用户的角色绑定
CREATE TABLE IF NOT EXISTS scim_group (
  id BIGSERIAL PRIMARY KEY,
  external_id VARCHAR(200) NOT NULL DEFAULT '',
  display_name VARCHAR(200) NOT NULL,
  members TEXT NOT NULL DEFAULT '[]',       -- JSON 数组：成员 value（SCIM user id 或 username）
  raw TEXT NOT NULL DEFAULT '{}',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_scim_group_name ON scim_group(display_name);
CREATE UNIQUE INDEX IF NOT EXISTS uk_scim_group_external ON scim_group(external_id) WHERE external_id <> '';

-- 组名 → 本系统角色编码映射。未配置映射时，组名本身若正好等于 rbac_role.code 也生效。
CREATE TABLE IF NOT EXISTS scim_role_map (
  id BIGSERIAL PRIMARY KEY,
  group_name VARCHAR(200) NOT NULL,
  role_code VARCHAR(50) NOT NULL,
  note VARCHAR(300) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_scim_role_map_group ON scim_role_map(group_name);
INSERT INTO scim_role_map(group_name, role_code, note) VALUES
  ('法务组', 'legal',    '示例映射：身份系统里的中文组名 → 本系统法务角色'),
  ('业务组', 'business', '示例映射：身份系统里的中文组名 → 本系统业务角色')
ON CONFLICT (group_name) DO NOTHING;

-- 每次推送的审计轨迹（不含 Bearer token）
CREATE TABLE IF NOT EXISTS scim_sync_log (
  id BIGSERIAL PRIMARY KEY,
  operation VARCHAR(30) NOT NULL DEFAULT '',
  resource_type VARCHAR(20) NOT NULL DEFAULT '',
  resource_id VARCHAR(100) NOT NULL DEFAULT '',
  username VARCHAR(100) NOT NULL DEFAULT '',
  external_id VARCHAR(200) NOT NULL DEFAULT '',
  http_status INT NOT NULL DEFAULT 0,
  message VARCHAR(500) NOT NULL DEFAULT '',
  actor VARCHAR(100) NOT NULL DEFAULT '',
  payload TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_scim_log_time ON scim_sync_log(created_at DESC);

-- 区分手工授权与 SCIM 自动授权：SCIM 同步只覆盖自己写的绑定，不冲掉管理员手工配的角色。
ALTER TABLE rbac_user_role ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'manual';
