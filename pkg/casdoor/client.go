package casdoor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// Config 对应 config.yaml 中的 app.casdoor
type Config struct {
	Endpoint     string
	ClientId     string
	ClientSecret string
	OrgName      string
	AppName      string
}

func LoadFromEnv() Config {
	return Config{
		Endpoint:     os.Getenv("CASDOOR_ENDPOINT"),
		ClientId:     os.Getenv("CASDOOR_CLIENT_ID"),
		ClientSecret: os.Getenv("CASDOOR_CLIENT_SECRET"),
		OrgName:      os.Getenv("CASDOOR_ORG_NAME"),
		AppName:      os.Getenv("CASDOOR_APP_NAME"),
	}
}

// ValidateToken 用 Casdoor Access Token 调 /api/userinfo 换取用户信息（生产态使用）。
// 需要一台运行中的 Casdoor 服务；当前骨架 devMode 下不调用。
func ValidateToken(cfg Config, token string) (map[string]interface{}, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("casdoor endpoint 未配置")
	}
	req, _ := http.NewRequest(http.MethodGet, cfg.Endpoint+"/api/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("casdoor 校验失败: %d", resp.StatusCode)
	}
	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return info, nil
}
