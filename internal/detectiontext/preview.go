// Package detectiontext derives bounded plain text from an already-reviewed
// privacy export. It never collects evidence, changes state or uses a clipboard.
package detectiontext

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Maoyangui/godusevpn/internal/detection"
)

const (
	maxInput    = 4 << 20
	maxOutput   = 256 << 10
	maxFindings = 512
)

var ErrPreview = errors.New("detection_text_reviewed_preview_refused")

type Preview struct {
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
	Count   int    `json:"count"`
}

type document struct {
	Schema           string              `json:"schema"`
	GeneratedAt      string              `json:"generatedAt"`
	Scope            string              `json:"scope"`
	Findings         []detection.Finding `json:"findings"`
	Warnings         []string            `json:"warnings"`
	Observer         *string             `json:"observer,omitempty"`
	Platform         *string             `json:"platform,omitempty"`
	ObserverElevated *bool               `json:"observerElevated,omitempty"`
	HelperAuthorized *bool               `json:"helperAuthorized,omitempty"`
}

// FromReviewed preserves the already selected findings and optional fields.
// It must be called only with backend-held preview bytes, never frontend JSON.
// Unknown/duplicate keys are refused so no hidden field is silently excluded.
func FromReviewed(data []byte) (Preview, error) {
	if len(data) == 0 || len(data) > maxInput || !utf8.Valid(data) {
		return Preview{}, ErrPreview
	}
	keys := json.NewDecoder(bytes.NewReader(data))
	if uniqueKeys(keys) != nil {
		return Preview{}, ErrPreview
	}
	if _, err := keys.Token(); err != io.EOF {
		return Preview{}, ErrPreview
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var doc document
	if decoder.Decode(&doc) != nil || doc.Schema != "godusevpn-ultra-privacy-export-v1" || doc.Scope != "all" && doc.Scope != "filtered" || len(doc.Findings) > maxFindings || len(doc.Warnings) > 64 {
		return Preview{}, ErrPreview
	}
	if _, err := time.Parse(time.RFC3339Nano, doc.GeneratedAt); err != nil {
		return Preview{}, ErrPreview
	}
	// Optional observation fields are one explicit selection in the existing
	// export contract. Never invent any of them for a default/minimal export.
	selectedObserver := doc.Observer != nil || doc.Platform != nil || doc.ObserverElevated != nil || doc.HelperAuthorized != nil
	if selectedObserver && (doc.Observer == nil || doc.Platform == nil || doc.ObserverElevated == nil || doc.HelperAuthorized == nil) {
		return Preview{}, ErrPreview
	}
	var text strings.Builder
	line := func(value string) error {
		if len(value) > 16384 || text.Len()+len(value)+1 > maxOutput {
			return ErrPreview
		}
		text.WriteString(value)
		text.WriteByte('\n')
		return nil
	}
	add := func(format string, args ...any) error { return line(fmt.Sprintf(format, args...)) }
	if line("Ultra 本地检测摘要") != nil || add("报告生成：%s", clean(doc.GeneratedAt)) != nil {
		return Preview{}, ErrPreview
	}
	scope := "当前筛选"
	if doc.Scope == "all" {
		scope = "完整报告"
	}
	if add("范围：%s · %d 项", scope, len(doc.Findings)) != nil || line("配置与有限观察不等于所有应用的实际防护；未知不视为安全。") != nil {
		return Preview{}, ErrPreview
	}
	if selectedObserver {
		answer := map[bool]string{false: "否", true: "是"}
		if add("观察身份：%s", clean(*doc.Observer)) != nil || add("观察平台：%s；观察进程提升：%s；助手授权：%s", clean(*doc.Platform), answer[*doc.ObserverElevated], answer[*doc.HelperAuthorized]) != nil {
			return Preview{}, ErrPreview
		}
	}
	statuses := map[string]string{"observed": "已观察到可见信息", "allowed": "配置允许读取", "possible": "仍有可能读取", "restricted": "检查范围内受限", "unknown": "无法确认", "manual": "需要手动检查"}
	sources := map[string]string{"policies": "系统策略", "firewall": "程序规则", "network": "服务状态", "facts": "只读环境事实", "browser-session": "本应用受控浏览器会话"}
	audiences := map[string]string{"website": "网站", "websites": "网站", "web": "网站", "browser": "网站", "app": "应用", "apps": "应用", "application": "应用", "native": "应用", "network": "网络", "admin": "管理员应用", "administrator": "管理员应用"}
	seen := map[string]bool{}
	for index, finding := range doc.Findings {
		status := statuses[finding.Status]
		if finding.ID == "" || len(finding.ID) > 128 || seen[finding.ID] || status == "" || len(finding.Title) == 0 || finding.Count != nil && *finding.Count < 0 {
			return Preview{}, ErrPreview
		}
		seen[finding.ID] = true
		stamp := "未提供；不补造来源时间"
		if finding.DetectedAt != "" {
			if _, err := time.Parse(time.RFC3339Nano, finding.DetectedAt); err != nil {
				return Preview{}, ErrPreview
			}
			stamp = clean(finding.DetectedAt)
		}
		source := sources[finding.Source]
		if source == "" {
			source = "未提供或未识别的本地来源"
		}
		audience := audiences[finding.Audience]
		if audience == "" {
			audience = "未分类"
		}
		if line("") != nil || add("%d. %s · %s", index+1, clean(finding.Title), status) != nil || add("   结果：%s", clean(finding.Summary)) != nil || add("   依据：%s", valueOr(finding.Evidence, "未提供可验证证据")) != nil || add("   范围：%s", valueOr(finding.Scope, "未确认")) != nil || add("   来源：%s；读取方：%s；读取完成：%s", source, audience, stamp) != nil {
			return Preview{}, ErrPreview
		}
		if finding.Count != nil && add("   数量：%d", *finding.Count) != nil {
			return Preview{}, ErrPreview
		}
	}
	if len(doc.Warnings) > 0 {
		if line("") != nil || line("报告限制：") != nil {
			return Preview{}, ErrPreview
		}
		for _, warning := range doc.Warnings {
			if add("- %s", clean(warning)) != nil {
				return Preview{}, ErrPreview
			}
		}
	}
	content := text.String()
	digest := sha256.Sum256([]byte(content))
	return Preview{Content: content, SHA256: hex.EncodeToString(digest[:]), Count: len(doc.Findings)}, nil
}

func valueOr(value, fallback string) string {
	if value = clean(value); value != "" {
		return value
	}
	return fallback
}

// Plain text cannot carry control/format characters to an eventual paste
// destination. The exact resulting text is what the UI must show before copy.
func clean(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func uniqueKeys(decoder *json.Decoder) error {
	return uniqueKeysAt(decoder, 0)
}

func uniqueKeysAt(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return ErrPreview
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				folded := strings.ToLower(name)
				if err != nil || !ok || seen[folded] {
					return ErrPreview
				}
				seen[folded] = true
				if uniqueKeysAt(decoder, depth+1) != nil {
					return ErrPreview
				}
			}
		case '[':
			for decoder.More() {
				if uniqueKeysAt(decoder, depth+1) != nil {
					return ErrPreview
				}
			}
		default:
			return ErrPreview
		}
		end, err := decoder.Token()
		if err != nil || delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
			return ErrPreview
		}
	}
	return nil
}
