package entity

import (
	"encoding/json"
	"time"
)

// PipelineStage 任务流水线阶段（ETL：解析 → AI 抽取 → 向量化[知识库阶段接入]）
type PipelineStage struct {
	Key      string `json:"key"`      // parse / extract / vectorize
	Label    string `json:"label"`    // 中文名
	Status   string `json:"status"`   // planned / running / done / failed
	Progress int    `json:"progress"` // 0-100
	Msg      string `json:"msg"`      // 可读说明（如 "3/12 块"）
}

// ContractFile 合同文件 + 抽取任务（AC1.1/1.3：上传 → 流水线状态机）
type ContractFile struct {
	Id            int64           `json:"id"`
	Filename      string          `json:"filename"`
	Path          string          `json:"path"`
	FileHash      string          `json:"fileHash"`
	Status        string          `json:"status"`        // pending / extracting / done / failed
	ErrorMsg      string          `json:"errorMsg"`      // 失败原因（AC1.6 可读报错）
	Stage         string          `json:"stage"`         // 当前阶段 key
	Progress      int             `json:"progress"`      // 总进度 0-100
	Pipeline      json.RawMessage `json:"pipeline"`      // 各阶段明细
	ReviewState   string          `json:"reviewState"`   // in_progress / complete（AC2.6 复核完成门槛）
	Template      string          `json:"template"`      // AC1.8 抽取模板：general/purchase/sales/lease
	GroupId       int64           `json:"groupId"`       // AC6.2 版本组：组内第一个文件的 id（0=自身即组首）
	VersionNo     int             `json:"versionNo"`     // AC6.2 组内版本号（1 起）
	ApprovalState string          `json:"approvalState"` // 审批状态：none/running/approved/rejected（AC6.4 联动）
	TextContent   string          `json:"-"`             // 解析后的合同全文（AC1.5 定位用；经 /files/{id}/text 单独下发，列表不带）
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// ContractClause 抽取出的条款条目（AC1.4 分类 / AC1.5 原文引用 / AC2.x 复核）
type ContractClause struct {
	Id           int64     `json:"id"`
	FileId       int64     `json:"fileId"`
	Type         string    `json:"type"`         // 主体/权利义务/金额/日期/违约/保密/争议解决/其他
	Summary      string    `json:"summary"`      // 条款要点
	Quote        string    `json:"quote"`        // 原文片段
	Risk         string    `json:"risk"`         // high/medium/low/none
	ReviewStatus string    `json:"reviewStatus"` // unconfirmed / confirmed / doubt（AC2.3）
	ReviewRemark string    `json:"reviewRemark"` // 疑义备注（AC2.3）
	CreatedAt    time.Time `json:"createdAt"`
}

// AuditLog 审计轨迹（AC6.5：谁+何时+何事；MVP 记录复核/修正/台账增删改）
type AuditLog struct {
	Id         int64                  `json:"id"`
	Username   string                 `json:"username"`
	Action     string                 `json:"action"` // clause.update / contract.create ...
	TargetType string                 `json:"targetType"`
	TargetId   int64                  `json:"targetId"`
	Detail     map[string]interface{} `json:"detail"`
	Ip         string                 `json:"ip"` // AK4.2 五要素：来源 IP
	CreatedAt  time.Time              `json:"createdAt"`
}

// SignTask 电子签任务（AC7.1-7.6：适配外部签提供方，MVP 内置 mock 演示状态机）
type SignTask struct {
	Id         int64     `json:"id"`
	FileId     int64     `json:"fileId"`
	Provider   string    `json:"provider"` // mock / docuseal / 商业CA（经 API 适配，不内嵌其代码）
	Status     string    `json:"status"`   // created / sent / signed / failed
	ExternalId string    `json:"externalId"`
	FailMsg    string    `json:"failMsg"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Obligation 义务清单（AC3.1-AC3.6：生成/分派/截止日/逾期/完成反馈）
type Obligation struct {
	Id           int64      `json:"id"`
	FileId       int64      `json:"fileId"`
	Type         string     `json:"type"`        // 付款 / 交付 / 报告 / 续约 / 其他
	Description  string     `json:"description"` // 谁+做什么+何时
	Owner        string     `json:"owner"`       // 负责人（AC3.2；Core 组织树就绪后改关联用户）
	DueDate      string     `json:"dueDate"`     // YYYY-MM-DD（AC3.3）
	SourceQuote  string     `json:"sourceQuote"` // 依据条款原文
	Status       string     `json:"status"`      // pending / done（AC3.6 完成反馈）
	Remark       string     `json:"remark"`
	EvidenceName string     `json:"evidenceName"` // AC3.6 证据附件名（文件存服务端）
	CompletedAt  *time.Time `json:"completedAt"`
	Overdue      bool       `json:"overdue"` // 计算字段（AC3.5 逾期）：pending 且 dueDate < 今天
	DueSoon      bool       `json:"dueSoon"` // 计算字段：7 天内到期
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// ModelConfig 对应 model_config 表（Core 共用内核：统一托管 AI/ML 接入配置）
type ModelConfig struct {
	Id          int64       `json:"id"`
	Name        string      `json:"name"`
	Category    string      `json:"category"` // llm / ocr / rerank / embedding / vector
	Provider    string      `json:"provider"`
	BaseUrl     string      `json:"baseUrl"`
	ModelName   string      `json:"modelName"`
	ApiKey      string      `json:"apiKey"` // 敏感字段，生产应加密/走 KMS，接口返回可按需脱敏
	ExtraConfig interface{} `json:"extraConfig"`
	Enabled     bool        `json:"enabled"`
	KeyUsable   bool        `json:"keyUsable"` // 密钥能否被当前进程解开（APP_AES_KEY 缺失时明确置 false）
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

// McpServer 外部 MCP server 连接配置（MCP Client：系统作为客户端去连接外部 server）
type McpServer struct {
	Id           int64     `json:"id"`
	Name         string    `json:"name"`
	Endpoint     string    `json:"endpoint"`
	Token        string    `json:"token"` // 敏感：存 enc:v1: 密文，接口返回脱敏
	ExtraHeaders string    `json:"extraHeaders"`
	Description  string    `json:"description"`
	Enabled      bool      `json:"enabled"`
	ToolCount    int       `json:"toolCount"`  // 最近拉取工具数（页面展示）
	LastStatus   string    `json:"lastStatus"` // idle / ok / error / pending
	LastError    string    `json:"lastError"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Contract 合同台账（产品① 合同线核心实体）
type Contract struct {
	Id           int64     `json:"id"`
	Title        string    `json:"title"`
	CounterParty string    `json:"counterParty"`
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	SignDate     string    `json:"signDate"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
