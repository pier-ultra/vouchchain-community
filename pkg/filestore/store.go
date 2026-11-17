package filestore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// 文件存储双后端（用户 11-15 拍板「兼容模式」）：
//   local（默认）：Go 单二进制自带，磁盘目录（现有 uploads/ 即历史根），零依赖装完即用
//   s3：外挂 SeaweedFS（S3 兼容），多机/大文件量时通过 FILE_STORE=s3 切换，代码不改
// 切换只认环境变量：FILE_STORE / S3_ENDPOINT / S3_BUCKET / S3_ACCESS_KEY / S3_SECRET_KEY

// Store 文件存储统一接口
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Kind() string // local / s3（审计与 file_entry.storage 记录用）
}

// 选后端：FILE_STORE=s3 且四项配置齐全 → S3（启动时探活，不可达回退 local 并告警）；否则 local
func Pick(ctx context.Context) Store {
	if strings.EqualFold(os.Getenv("FILE_STORE"), "s3") {
		endpoint := os.Getenv("S3_ENDPOINT")
		bucket := os.Getenv("S3_BUCKET")
		ak := os.Getenv("S3_ACCESS_KEY")
		sk := os.Getenv("S3_SECRET_KEY")
		if endpoint == "" || bucket == "" || ak == "" || sk == "" {
			glog.Warning(ctx, "[filestore] FILE_STORE=s3 但 S3_ENDPOINT/S3_BUCKET/S3_ACCESS_KEY/S3_SECRET_KEY 未配齐，回退本地磁盘存储")
		} else {
			s, err := newS3Store(ctx, endpoint, bucket, ak, sk)
			if err != nil {
				glog.Warningf(ctx, "[filestore] S3(%s) 连接失败：%v，回退本地磁盘存储", endpoint, err)
			} else {
				glog.Infof(ctx, "[filestore] 使用 SeaweedFS/S3 存储：endpoint=%s bucket=%s", endpoint, bucket)
				return s
			}
		}
	}
	root := os.Getenv("FILE_LOCAL_DIR")
	if root == "" {
		root = "uploads"
	}
	glog.Infof(ctx, "[filestore] 使用本地磁盘存储：root=%s", root)
	return &LocalStore{Root: root}
}

// ---------- 本地磁盘后端 ----------

type LocalStore struct {
	Root string
}

func (s *LocalStore) Kind() string { return "local" }

func (s *LocalStore) path(key string) (string, error) {
	clean := filepath.Clean("/" + key) // 防目录穿越
	p := filepath.Join(s.Root, clean)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, nil
}

func (s *LocalStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func (s *LocalStore) Get(ctx context.Context, key string) ([]byte, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ---------- S3/SeaweedFS 后端 ----------

type S3Store struct {
	cli    *minio.Client
	bucket string
}

func (s *S3Store) Kind() string { return "s3" }

func newS3Store(ctx context.Context, endpoint, bucket, ak, sk string) (*S3Store, error) {
	secure := !strings.Contains(endpoint, ":8333") && !strings.HasPrefix(endpoint, "http://")
	// SeaweedFS S3 默认 http；显式 http:// 前缀则强制非 TLS
	if strings.HasPrefix(endpoint, "http://") {
		secure = false
		endpoint = strings.TrimPrefix(endpoint, "http://")
	} else if strings.HasPrefix(endpoint, "https://") {
		secure = true
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(ak, sk, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, err
	}
	exists, err := cli.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket 探活失败: %w", err)
	}
	if !exists {
		if err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("建 bucket 失败: %w", err)
		}
		glog.Infof(ctx, "[filestore] 已创建 bucket %s", bucket)
	}
	return &S3Store{cli: cli, bucket: bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.cli.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.cli.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	return s.cli.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// 说明：历史 uploads/ 文件在 local 模式下天然可用（Root 即 uploads/）；
// 切 s3 前用迁移命令把本地文件灌入（后续提供 filestore migrate 子命令）。
var _ = g.Log // 保持 g 包引用（glog 已用）
