package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// 模型 API Key 的库内加密存储：
// - 主密钥来自环境变量 APP_AES_KEY（任意长度字符串，内部 SHA-256 派生 32 字节 AES-256 密钥）
// - 密文格式：enc:v1:<base64(nonce+ciphertext)>（AES-256-GCM，自带完整性校验）
// - 未加前缀的值视为历史明文，Decrypt 直接透传（兼容）
// - 列表/查询接口一律返回 Mask 后的脱敏值，密文不出库、明文不进 API 响应

const encPrefix = "enc:v1:"

func masterKey() []byte {
	s := os.Getenv("APP_AES_KEY")
	if s == "" {
		return nil
	}
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// Encrypt 明文 → enc:v1: 密文；未配置 APP_AES_KEY 时报错（调用方提示用户配置）
func Encrypt(plain string) (string, error) {
	key := masterKey()
	if key == nil {
		return "", errors.New("APP_AES_KEY 未配置，无法加密（服务端需先设置该环境变量）")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// Decrypt enc:v1: 密文 → 明文；非加密前缀的值按历史明文透传
func Decrypt(stored string) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	key := masterKey()
	if key == nil {
		return "", errors.New("APP_AES_KEY 未配置，无法解密")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("密文长度异常")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败（密钥不匹配或数据损坏）: %w", err)
	}
	return string(pt), nil
}

// Mask 脱敏展示：密文显示占位，明文只露头尾
func Mask(stored string) string {
	if stored == "" {
		return ""
	}
	if strings.HasPrefix(stored, encPrefix) {
		return "已加密存储"
	}
	if len(stored) <= 8 {
		return "••••••••"
	}
	return stored[:4] + "••••" + stored[len(stored)-4:]
}
