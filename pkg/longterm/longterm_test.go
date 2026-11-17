package longterm

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestSM3Digest(t *testing.T) {
	d1 := SM3Digest([]byte("abc"))
	d2 := SM3Digest([]byte("abc"))
	d3 := SM3Digest([]byte("abd"))
	if d1 != d2 {
		t.Errorf("同输入摘要应一致: %s vs %s", d1, d2)
	}
	if d1 == d3 {
		t.Errorf("不同输入摘要应不同")
	}
	if len(d1) != 64 { // SM3 = 32 字节 = 64 hex
		t.Errorf("SM3 摘要应 64 hex 字符, got %d", len(d1))
	}
	// 已知 SM3("abc") = 66c7f0f4...（国密标准向量）
	known := "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"
	if d1 != known {
		t.Errorf("SM3(\"abc\") 应匹配国密标准向量:\n got %s\nwant %s", d1, known)
	}
}

func TestSM4RoundTrip(t *testing.T) {
	key, _ := hex.DecodeString("0123456789abcdeffedcba9876543210")
	plain := []byte("合同归档件长期保存加密测试-2026")
	enc, err := SM4Encrypt(plain, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(enc, plain) {
		t.Errorf("密文不应包含明文")
	}
	dec, err := SM4Decrypt(enc, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Errorf("解密不一致:\n got %q\nwant %q", dec, plain)
	}
	// 空数据
	enc0, _ := SM4Encrypt([]byte(""), key)
	dec0, err := SM4Decrypt(enc0, key)
	if err != nil || len(dec0) != 0 {
		t.Errorf("空数据回环失败: %v", err)
	}
	// 错误密钥解不出
	bad, _ := hex.DecodeString("ffffffffffffffffffffffffffffffff")
	if _, err := SM4Decrypt(enc, bad); err == nil {
		t.Errorf("错误密钥不应成功解密（应填充校验失败或内容错）")
	}
}
