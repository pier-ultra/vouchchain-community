package longterm

import (
	"context"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/tjfoc/gmsm/sm3"
	"github.com/tjfoc/gmsm/sm4"
)

// A5 长期保存/国密合规封装层（P2，2028-01-01 财会 9 号硬窗口）：
//   - SM3 摘要：国密哈希，防篡改（不自研算法，用 tjfoc/gmsm）。
//   - SM4-CBC 加密：归档件加密落盘（不自研算法，用 gmsm/sm4；IV 由密钥派生，生产应由 LONGTERM_SM4_KEY 提供 16 字节密钥）。
//   - 时间戳登记：归档时记录可信时间（系统时间 + 摘要绑定，形成存证记录）。
//   - 版本控制：同源件重复归档自动追加版本（v1/v2/...，保留全部历史）。
// 边界：不做异地容灾多副本（依赖外部存储配置，说明见 README），只做单库加密存证 + 可校验。

// SM3Digest 国密 SM3 摘要（hex）。不自研算法。
func SM3Digest(data []byte) string {
	h := sm3.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// sm4Key 取 16 字节 SM4 密钥：优先环境变量 LONGTERM_SM4_KEY（hex 32 字符），否则内置开发密钥。
// 生产环境必须配置 LONGTERM_SM4_KEY。
func sm4Key() ([]byte, error) {
	if k := strings.TrimSpace(os.Getenv("LONGTERM_SM4_KEY")); k != "" {
		if b, err := hex.DecodeString(k); err == nil && len(b) == 16 {
			return b, nil
		}
		return nil, fmt.Errorf("LONGTERM_SM4_KEY 需为 16 字节的 hex（32 字符）")
	}
	// 开发默认密钥（SM3 派生固定串前 16 字节）——仅本地演示，生产务必配置环境变量
	d := SM3Digest([]byte("contract-hub-dev-sm4-key"))
	return []byte(d[:16]), nil
}

// SM4Encrypt SM4-CBC 加密（IV = SM3(key) 前 16 字节，密钥 16 字节）
func SM4Encrypt(data []byte, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("SM4 密钥需 16 字节，got %d", len(key))
	}
	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := []byte(SM3Digest(key))[:16]
	// PKCS7 填充
	padding := block.BlockSize() - len(data)%block.BlockSize()
	padded := append(append([]byte{}, data...), make([]byte, padding)...)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out, nil
}

// SM4Decrypt SM4-CBC 解密（配套 SM4Encrypt）
func SM4Decrypt(data []byte, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("SM4 密钥需 16 字节，got %d", len(key))
	}
	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := []byte(SM3Digest(key))[:16]
	if len(data) == 0 || len(data)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("密文长度非法")
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	// 去 PKCS7 填充
	pad := int(out[len(out)-1])
	if pad < 1 || pad > block.BlockSize() || pad > len(out) {
		return nil, fmt.Errorf("填充校验失败")
	}
	return out[:len(out)-pad], nil
}

// Archive 归档长期保存：SM3 摘要 + SM4 加密落盘（版本化）+ 时间戳登记。
// 返回存证记录（含版本号/摘要/时间戳）。
func Archive(ctx context.Context, sourceType string, sourceID int64, srcPath, remark string) (g.Map, error) {
	if sourceType == "" || sourceID <= 0 {
		return nil, fmt.Errorf("sourceType 与 sourceID 必填")
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("读取源件失败: %w", err)
	}
	key, err := sm4Key()
	if err != nil {
		return nil, err
	}
	enc, err := SM4Encrypt(data, key)
	if err != nil {
		return nil, err
	}
	digest := SM3Digest(data)

	// 版本控制：查已有最大版本
	var maxVer int
	if row, err := g.DB().Model("longterm_archive").
		Where("source_type", sourceType).Where("source_id", sourceID).Order("version desc").One(); err == nil && !row.IsEmpty() {
		maxVer = row["version"].Int()
	}
	ver := maxVer + 1

	// 加密件落盘（归档目录按 类型-源id 分桶，版本化保留历史）
	dir := filepath.Join(os.Getenv("LONGTERM_DIR"), fmt.Sprintf("%s-%d", sourceType, sourceID))
	if dir == "" || strings.HasPrefix(dir, "-") {
		dir = filepath.Join("uploads", "longterm", fmt.Sprintf("%s-%d", sourceType, sourceID))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	encPath := filepath.Join(dir, fmt.Sprintf("v%d.bin", ver))
	if err := os.WriteFile(encPath, enc, 0o644); err != nil {
		return nil, err
	}

	ts := time.Now().Format("2006-01-02 15:04:05")
	res, err := g.DB().Model("longterm_archive").Data(g.Map{
		"source_type": sourceType, "source_id": sourceID, "version": ver,
		"digest_alg": "sm3", "digest": digest, "encrypt_alg": "sm4",
		"enc_path": encPath, "ts_token": digest[:32], "ts_time": ts,
		"remark": remark, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return g.Map{"id": id, "version": ver, "digest": digest, "ts_time": ts, "enc_path": encPath}, nil
}

// List 长期保存登记列表
func List(ctx context.Context, sourceType string, limit int) ([]g.Map, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	m := g.DB().Model("longterm_archive")
	if sourceType != "" {
		m = m.Where("source_type", sourceType)
	}
	rows, err := m.Order("id desc").Limit(limit).All()
	if err != nil {
		return nil, err
	}
	out := make([]g.Map, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Map())
	}
	return out, nil
}

// Verify 防篡改校验：重算源件 SM3 摘要与登记比对（源件可读时）；并校验加密件可解出原文。
func Verify(ctx context.Context, id int64, srcPath string) (g.Map, error) {
	row, err := g.DB().Model("longterm_archive").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		return nil, fmt.Errorf("登记记录不存在")
	}
	reg := row.Map()
	_ = reg
	res := g.Map{"id": id, "digestOk": nil, "decryptOk": nil, "msg": ""}
	// 1) 源件摘要比对
	if srcPath != "" {
		if data, err := os.ReadFile(srcPath); err == nil {
			res["digestOk"] = SM3Digest(data) == row["digest"].String()
		}
	}
	// 2) 加密件可解出原文（防篡改 + 可用性）
	if key, err := sm4Key(); err == nil {
		if enc, err := os.ReadFile(row["enc_path"].String()); err == nil {
			if dec, err := SM4Decrypt(enc, key); err == nil {
				res["decryptOk"] = SM3Digest(dec) == row["digest"].String()
			}
		}
	}
	res["msg"] = "已校验（digestOk/decryptOk 见字段）"
	return res, nil
}
