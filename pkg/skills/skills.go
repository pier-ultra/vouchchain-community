package skills

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

// Agent Skill 包管理（用户 11-15 设计拍板）：
//   磁盘 skills/{name}/ 是权威源（升级存活、离线可用、加载零依赖）；
//   上传的 zip 原件登记进文件中心（file_entry，biz_type=skill）做备份/分享——双后端（local/SeaweedFS）自动生效。

type Skill struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Triggers    string `json:"triggers"`
	Version     string `json:"version"`
	Enabled     bool   `json:"enabled"`
	Path        string `json:"path"`
}

const skillsRoot = "skills"

// 技能名白名单：仅字母/数字/下划线/连字符，1-64 位（防 name 拼路径穿越）
var skillNameRe = regexp.MustCompile(`^[A-Za-z0-9_\-]{1,64}$`)

// zip 内合法成员路径（防 zip-slip）：先查原始路径再规范化，
// 拒绝 ..、NUL、绝对路径、盘符；返回以 / 分隔的相对路径
func safeZipPath(name string) (string, error) {
	if strings.ContainsRune(name, 0) || strings.Contains(name, "..") {
		return "", fmt.Errorf("zip 内含非法路径: %s", name)
	}
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if clean == "." || strings.HasPrefix(clean, "/") || (len(clean) >= 2 && clean[1] == ':') {
		return "", fmt.Errorf("zip 内含非法路径: %s", name)
	}
	return clean, nil
}

// installFromBytes 解压 zip（不落盘到临时文件：直接流式解）到 skills/{name}/
func installFromBytes(data []byte) (name, description, triggers, instructions string, err error) {
	zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return "", "", "", "", fmt.Errorf("不是有效的 zip 包: %w", err)
	}
	// 找 SKILL.md（根目录或单层目录内）
	var skillFile *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if base == "SKILL.md" || base == "skill.md" {
			skillFile = f
			break
		}
	}
	if skillFile == nil {
		return "", "", "", "", errors.New("包内未找到 SKILL.md（技能包需在根目录或一级子目录含 SKILL.md）")
	}
	// 解析 frontmatter（--- 包围的 YAML 简解析：name/description/triggers/version）
	rc, err := skillFile.Open()
	if err != nil {
		return "", "", "", "", err
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	name, description, triggers, instructions = parseSkillMD(string(body))
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filepath.Dir(skillFile.Name)), "/")
	}
	if name == "" || name == "." {
		return "", "", "", "", errors.New("SKILL.md 未声明 name 且无法从目录推断")
	}
	if !skillNameRe.MatchString(name) {
		return "", "", "", "", errors.New("SKILL.md 的 name 含非法字符（仅允许字母/数字/下划线/连字符，≤64 位）")
	}
	// 解压全部文件到 skills/{name}/
	root := filepath.Join(skillsRoot, name)
	rootPrefix := filepath.Clean(root) + string(os.PathSeparator)
	for _, f := range zr.File {
		p, perr := safeZipPath(f.Name)
		if perr != nil {
			return "", "", "", "", perr
		}
		// 去掉包内一层公共目录（若有）
		rel := p
		parts := strings.SplitN(p, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[0] != name {
			rel = parts[1]
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(filepath.Join(root, rel), 0o755)
			continue
		}
		dst := filepath.Join(root, rel)
		// 解压目标必须仍在 skills/{name}/ 内（防 zip-slip 双保险）
		if !strings.HasPrefix(filepath.Clean(dst)+string(os.PathSeparator), rootPrefix) {
			return "", "", "", "", fmt.Errorf("zip 内含非法路径: %s", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", "", "", "", err
		}
		fr, ferr := f.Open()
		if ferr != nil {
			return "", "", "", "", ferr
		}
		fw, ferr := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if ferr != nil {
			fr.Close()
			return "", "", "", "", ferr
		}
		_, ferr = io.Copy(fw, fr)
		fr.Close()
		fw.Close()
		if ferr != nil {
			return "", "", "", "", ferr
		}
	}
	return name, description, triggers, instructions, nil
}

// parseSkillMD 解析 SKILL.md：YAML frontmatter 的 name/description/triggers/version + 正文为 instructions
func parseSkillMD(content string) (name, description, triggers, instructions string) {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inFm := true
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "---" {
				instructions = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
				break
			}
			if !inFm {
				continue
			}
			kv := strings.SplitN(line, ":", 2)
			if len(kv) != 2 {
				continue
			}
			k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(strings.Trim(kv[1], `"'`))
			switch k {
			case "name":
				name = v
			case "description":
				description = v
			case "triggers":
				triggers = v
			case "version":
				// 保留展示
			}
		}
		if instructions == "" && strings.TrimSpace(lines[len(lines)-1]) != "---" {
			instructions = strings.TrimSpace(content)
		}
	} else {
		instructions = strings.TrimSpace(content)
	}
	return name, description, triggers, instructions
}

// Install 安装技能包：解压落盘（权威源）+ 入库；调用方负责把 zip 原件登记进文件中心
func Install(ctx context.Context, creator string, data []byte) (int64, error) {
	name, description, triggers, instructions, err := installFromBytes(data)
	if err != nil {
		return 0, err
	}
	exist, _ := g.DB().Model("skill_pack").Where("name", name).One()
	data2 := g.Map{
		"name": name, "description": description, "triggers": triggers,
		"instructions": instructions, "path": filepath.Join(skillsRoot, name),
		"creator": creator, "enabled": true,
	}
	if exist.IsEmpty() {
		res, err := g.DB().Model("skill_pack").Data(data2).Insert()
		if err != nil {
			return 0, err
		}
		id, _ := res.LastInsertId()
		return id, nil
	}
	// 同名覆盖升级
	if _, err := g.DB().Model("skill_pack").Data(data2).Where("id", exist["id"].Int64()).Update(); err != nil {
		return 0, err
	}
	return exist["id"].Int64(), nil
}

// MatchPrompt 按 triggers 命中已启用技能，拼接注入 prompt（无命中返回空串）
func MatchPrompt(ctx context.Context, userMsg string) string {
	rows, err := g.DB().Model("skill_pack").Where("enabled", true).All()
	if err != nil || len(rows) == 0 {
		return ""
	}
	attached := make([]string, 0, 2)
	for _, r := range rows {
		kws := strings.Split(r["triggers"].String(), ",")
		hit := false
		for _, kw := range kws {
			kw = strings.TrimSpace(kw)
			if kw != "" && strings.Contains(userMsg, kw) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		attached = append(attached, fmt.Sprintf("【技能：%s】%s\n%s",
			r["display_name"].String(), r["description"].String(), r["instructions"].String()))
	}
	if len(attached) == 0 {
		return ""
	}
	return "\n\n以下为按用户消息匹配到的技能指令，请遵循：\n" + strings.Join(attached, "\n\n")
}

// List 全部技能
func List(ctx context.Context) ([]Skill, error) {
	rows, err := g.DB().Model("skill_pack").Order("id asc").All()
	if err != nil {
		return nil, err
	}
	out := make([]Skill, 0, len(rows))
	for _, r := range rows {
		out = append(out, Skill{
			ID: r["id"].Int64(), Name: r["name"].String(),
			DisplayName: r["display_name"].String(), Description: r["description"].String(),
			Triggers: r["triggers"].String(), Version: r["version"].String(),
			Enabled: r["enabled"].Bool(), Path: r["path"].String(),
		})
	}
	return out, nil
}
