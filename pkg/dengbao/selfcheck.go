package dengbao

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// ---------- R-C7：GB/T 22239-2019 合规自查清单 ----------
// 口径：这是「自评」，帮客户在测评前先对一遍标准控制点、把证据链挂上；
// 不替代测评机构现场测评，结果不宣称"已通过等保"。

// 自查结果取值（导出中文对照见 controller 层）
const (
	SelfcheckTodo    = "todo"    // 未自查
	SelfcheckYes     = "y"       // 符合
	SelfcheckNo      = "n"       // 不符合
	SelfcheckPartial = "partial" // 部分符合
	SelfcheckNA      = "na"      // 不适用
)

// ValidSelfcheckResult 纯函数：自查结果取值是否合法。
func ValidSelfcheckResult(s string) bool {
	switch s {
	case SelfcheckTodo, SelfcheckYes, SelfcheckNo, SelfcheckPartial, SelfcheckNA, "":
		return true
	}
	return false
}

// Selfcheck 自查项输入
type Selfcheck struct {
	ID       int64
	Code     string // 控制点编号（8.1.4.3 等）
	Item     string // 控制点名称/要求
	Domain   string // 安全控制族
	Level    string // 适用级别 1/2/3
	Result   string // todo/y/n/partial/na
	Evidence string // 证据链接或说明
	Note     string
}

// SelfcheckSave 登记/更新自查项（id>0 更新，否则新建）
func SelfcheckSave(ctx context.Context, s Selfcheck, by string) (int64, error) {
	if strings.TrimSpace(s.Item) == "" {
		return 0, fmt.Errorf("控制点名称必填")
	}
	if !ValidSelfcheckResult(s.Result) {
		return 0, fmt.Errorf("自查结果取值不合法：%s", s.Result)
	}
	if s.Result == "" {
		s.Result = SelfcheckTodo
	}
	if s.Level == "" {
		s.Level = "3"
	}
	data := g.Map{
		"code": s.Code, "item": s.Item, "domain": s.Domain, "level": s.Level,
		"result": s.Result, "evidence": s.Evidence, "note": s.Note,
		"updated_at": time.Now(),
	}
	if s.Result != SelfcheckTodo {
		data["checked_by"] = by
		data["checked_at"] = time.Now()
	}
	if s.ID > 0 {
		row, err := g.DB().Model("dengbao_selfcheck").Where("id", s.ID).One()
		if err != nil {
			return 0, err
		}
		if row.IsEmpty() {
			return 0, fmt.Errorf("自查项不存在")
		}
		if _, err := g.DB().Model("dengbao_selfcheck").Data(data).Where("id", s.ID).Update(); err != nil {
			return 0, err
		}
		return s.ID, nil
	}
	res, err := g.DB().Model("dengbao_selfcheck").Data(g.Map{
		"code": s.Code, "item": s.Item, "domain": s.Domain, "level": s.Level,
		"result": s.Result, "evidence": s.Evidence, "note": s.Note,
		"checked_by": by, "checked_at": time.Now(),
		"created_at": time.Now(), "updated_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// SelfcheckList 自查清单（domain/result 过滤）
func SelfcheckList(ctx context.Context, domain, result string) ([]gdb.Record, error) {
	m := g.DB().Model("dengbao_selfcheck")
	if domain != "" {
		m = m.Where("domain", domain)
	}
	if result != "" {
		m = m.Where("result", result)
	}
	return m.Order("domain asc, code asc, id asc").Limit(500).All()
}

// SelfcheckDelete 删除自查项
func SelfcheckDelete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("自查项 ID 必填")
	}
	_, err := g.DB().Model("dengbao_selfcheck").Where("id", id).Delete()
	return err
}

// SelfcheckStats 自查通过率统计（分母不含「不适用」）
func SelfcheckStats(ctx context.Context) (map[string]any, error) {
	rows, err := g.DB().GetAll(ctx, `
		SELECT result, COUNT(*) AS n FROM dengbao_selfcheck GROUP BY result`)
	if err != nil {
		return nil, err
	}
	by := map[string]int64{}
	total := int64(0)
	for _, r := range rows {
		n := r["n"].Int64()
		by[r["result"].String()] = n
		total += n
	}
	// 适用项 = 总数 − 不适用（通过率口径）
	applied := total - by[SelfcheckNA]
	passRate := 0.0
	if applied > 0 {
		passRate = float64(by[SelfcheckYes]) / float64(applied) * 100
	}
	return map[string]any{
		"total": total, "applied": applied,
		"yes": by[SelfcheckYes], "no": by[SelfcheckNo],
		"partial": by[SelfcheckPartial], "todo": by[SelfcheckTodo],
		"na": by[SelfcheckNA], "passRate": passRate,
	}, nil
}

// SelfcheckTemplate 三级等保主要控制点自查模板（首次使用一键铺底；
// 条目按 GB/T 22239-2019 8.1 安全通用要求的安全控制族抽选，不是全部测评项）。
type SelfcheckTemplate struct {
	Code   string
	Domain string
	Item   string
}

var selfcheckTemplate = []SelfcheckTemplate{
	{"8.1.1.1", "安全物理环境", "应对机房出入口设置访问控制措施，并对来访进行登记与审批"},
	{"8.1.1.2", "安全物理环境", "应设置自动消防与温湿度监控设施，防火防雷防水达标"},
	{"8.1.2.1", "安全通信网络", "应保证网络设备的业务处理能力满足业务高峰期需要，具备冗余"},
	{"8.1.2.2", "安全通信网络", "应采用密码技术保证通信过程中数据的完整性与保密性"},
	{"8.1.3.1", "安全区域边界", "应保证跨越安全边界的访问和数据流通过受控接口通信"},
	{"8.1.3.2", "安全区域边界", "应对进出网络的信息内容进行过滤，限制仅授权地址可访问重要主机"},
	{"8.1.3.3", "安全区域边界", "应能检测出常见的网络入侵行为并及时告警，对边界与重要节点进行监测"},
	{"8.1.3.5", "安全区域边界", "应在网络边界与重要网络节点处检测、清除恶意代码"},
	{"8.1.4.1", "安全计算环境", "应对外部提供的远程管理服务启用双因子认证；能检测并清除非法登录会话"},
	{"8.1.4.2", "安全计算环境", "应对登录账户分配最小必要权限；默认账户非必要时删除或锁定并限制默认组权限"},
	{"8.1.4.3", "安全计算环境", "应启用安全审计，覆盖到每个用户，对重要用户行为与安全事件留痕，日志留存不少于 6 个月"},
	{"8.1.4.4", "安全计算环境", "应能发现常见入侵行为并及时告警，检测到入侵时能终止相关行为"},
	{"8.1.4.5", "安全计算环境", "应采用恶意代码防范软件并及时更新，检测到恶意代码时自动告警"},
	{"8.1.4.6", "安全计算环境", "应通过可信计算技术从最小可信根构建验证链路（未引入可信计算环境可标不适用）"},
	{"8.1.4.7", "安全计算环境", "应采取校验技术保证重要数据处理过程中的完整性"},
	{"8.1.4.8", "安全计算环境", "应采用密码技术保证重要数据传输过程的保密性"},
	{"8.1.4.9", "安全计算环境", "应重要数据异地备份存放，并提供自动重启与冗余机制，定期恢复演练"},
	{"8.1.4.10", "安全计算环境", "应保证鉴别信息所在存储空间被释放或重分配前得到完全清除"},
	{"8.1.4.11", "安全计算环境", "应仅采集与保存业务必需的最少个人信息，禁止未授权访问与使用"},
	{"8.1.5.1", "安全管理中心", "应对系统管理员进行双因子身份鉴别，仅允许通过特定终端或通道登录"},
	{"8.1.5.2", "安全管理中心", "应指派专人分析审计日志，并能根据审计日志提供报警功能"},
	{"8.1.5.3", "安全管理中心", "应对安全管理员进行双因子身份鉴别，仅允许通过特定终端或通道登录"},
	{"8.1.5.4", "安全管理中心", "应集中监测网络与设备运行状况、恶意代码与审计状况，并统一纳管网络设备与安全设备"},
}

// SelfcheckInit 首次铺底：表为空时写入三级等保自查模板（幂等，有数据不动）。
func SelfcheckInit(ctx context.Context) (int64, error) {
	cnt, err := g.DB().Model("dengbao_selfcheck").Count()
	if err != nil {
		return 0, err
	}
	if cnt > 0 {
		return 0, nil
	}
	now := time.Now()
	for _, t := range selfcheckTemplate {
		_, err = g.DB().Model("dengbao_selfcheck").Data(g.Map{
			"code": t.Code, "item": t.Item, "domain": t.Domain, "level": "3",
			"result": SelfcheckTodo, "created_at": now, "updated_at": now,
		}).Insert()
		if err != nil {
			return 0, err
		}
	}
	return int64(len(selfcheckTemplate)), nil
}
