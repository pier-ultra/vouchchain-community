package seal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// R-B11 电子签平台盖章记录回填：
// 电子真章不做（CA 资质墙），只适配外部电子签平台的盖章记录回填用印台账。
// 红线：未配置外部平台 / 平台无返回时明确失败，不谎报「已同步」。

// ESNotConfiguredErr 未配置外部平台时的明确错误（不静默、不兜底假数据）
const ESNotConfiguredErr = "未配置外部电子签平台地址（seal_es_config.endpoint），无法回填；请先在「电子签配置」填写平台查询地址并启用"

// ESConfig 外部电子签平台配置
type ESConfig struct {
	Platform string
	Endpoint string
	Token    string // 读接口不回传
	Enabled  string // on/off
	Remark   string
}

// ESConfigGet 读配置（token 脱敏，避免经接口/日志外泄）
func ESConfigGet(ctx context.Context) (ESConfig, error) {
	row, err := g.DB().Model("seal_es_config").Where("id", 1).One()
	if err != nil {
		return ESConfig{}, err
	}
	if row.IsEmpty() {
		return ESConfig{}, nil
	}
	return ESConfig{
		Platform: row["platform"].String(),
		Endpoint: row["endpoint"].String(),
		Token:    "******",
		Enabled:  row["enabled"].String(),
		Remark:   row["remark"].String(),
	}, nil
}

// ESConfigSave 保存配置。token 传空表示不改凭据（接口永不回传原值）
func ESConfigSave(ctx context.Context, cfg ESConfig, by string) error {
	if cfg.Enabled != "on" && cfg.Enabled != "off" {
		return fmt.Errorf("启用状态取值不合法：%s", cfg.Enabled)
	}
	data := g.Map{
		"platform": cfg.Platform, "endpoint": strings.TrimSpace(cfg.Endpoint),
		"enabled": cfg.Enabled, "remark": cfg.Remark,
		"updated_by": by, "updated_at": time.Now(),
	}
	// token 传空 / 传占位符 = 不改凭据（更新语句不含 token 列，保留库内原值）
	if strings.TrimSpace(cfg.Token) != "" && cfg.Token != "******" {
		data["token"] = cfg.Token
	}
	res, err := g.DB().Model("seal_es_config").Data(data).Where("id", 1).Update()
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// 单行配置缺失时补建（迁移器未跑到的兜底）
		_, err = g.DB().Model("seal_es_config").Data(g.Map{
			"id": 1, "platform": cfg.Platform, "endpoint": strings.TrimSpace(cfg.Endpoint),
			"token": cfg.Token, "enabled": cfg.Enabled, "remark": cfg.Remark,
			"updated_by": by, "updated_at": time.Now(),
		}).Insert()
		return err
	}
	return nil
}

// esResponse 外部电子签平台盖章记录返回（字段命名兼容 camel/snake）
type esResponse struct {
	Ref         string `json:"ref"`
	EsFileNo    string `json:"esFileNo"`
	Signer      string `json:"signer"`
	SignedAt    string `json:"signedAt"`
	EvidenceURL string `json:"evidenceUrl"`
}

// ESSync R-B11：按用印申请从外部平台拉取盖章记录并回填台账（未配置明确失败）
func ESSync(ctx context.Context, applyID int64, by string) (int64, error) {
	if applyID <= 0 {
		return 0, fmt.Errorf("用印申请 ID 必填")
	}
	apply, err := g.DB().Model("seal_apply").Where("id", applyID).One()
	if err != nil {
		return 0, err
	}
	if apply.IsEmpty() {
		return 0, fmt.Errorf("用印申请不存在：%d", applyID)
	}
	cfg, err := ESConfigGet(ctx)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return 0, fmt.Errorf(ESNotConfiguredErr)
	}
	if cfg.Enabled != "on" {
		return 0, fmt.Errorf("电子签回填未启用（seal_es_config.enabled=off），先在「电子签配置」启用")
	}

	body, _ := json.Marshal(map[string]any{"applyId": applyID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("组装外部请求失败：%w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" && cfg.Token != "******" {
		req.Header.Set("X-ES-Token", cfg.Token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("调用外部电子签平台失败：%w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("外部电子签平台返回 HTTP %d：%s", resp.StatusCode, truncate(string(raw), 200))
	}
	var got esResponse
	if err := json.Unmarshal(raw, &got); err != nil {
		return 0, fmt.Errorf("解析外部返回失败：%w", err)
	}
	if strings.TrimSpace(got.EsFileNo) == "" {
		return 0, fmt.Errorf("外部平台未返回电子章编号，无法回填（原始返回：%s）", truncate(string(raw), 200))
	}
	signedAt := time.Time{}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, e := time.ParseInLocation(layout, strings.TrimSpace(got.SignedAt), time.Local); e == nil {
			signedAt = t
			break
		}
	}

	// 回填用印台账（电子章四要素）
	if _, err := g.DB().Model("seal_apply").Data(g.Map{
		"es_platform": cfg.Platform, "es_file_no": got.EsFileNo,
		"es_signer": got.Signer, "es_signed_at": signedAt,
	}).Where("id", applyID).Update(); err != nil {
		return 0, err
	}
	// 留痕（每次同步一条，raw 不含凭据）
	res, err := g.DB().Model("seal_es_record").Data(g.Map{
		"apply_id": applyID, "platform": cfg.Platform, "ext_ref": got.Ref,
		"es_file_no": got.EsFileNo, "signer": got.Signer, "signed_at": signedAt,
		"evidence_url": got.EvidenceURL, "raw": truncate(string(raw), 4000),
		"creator": by, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// ESRecordList 回填记录列表
func ESRecordList(ctx context.Context, applyID int64) ([]gdb.Record, error) {
	m := g.DB().Model("seal_es_record")
	if applyID > 0 {
		m = m.Where("apply_id", applyID)
	}
	return m.Order("id desc").Limit(200).All()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
