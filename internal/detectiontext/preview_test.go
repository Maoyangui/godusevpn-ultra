package detectiontext

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/detection"
)

func fixtureDocument() document {
	return document{Schema: "godusevpn-ultra-privacy-export-v1", GeneratedAt: "2026-10-27T01:00:00Z", Scope: "filtered", Findings: []detection.Finding{{ID: "network-path", Title: "隧道与直连范围", Status: "unknown", Summary: "本次没有独立流量证据。", Evidence: "仅服务状态", Scope: "当前观察范围", Source: "network", Audience: "network"}}, Warnings: []string{"配置不等于实际效果。"}}
}
func encodeDocument(t *testing.T, doc document) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestTextPreservesSelectedUnknownEvidenceWithoutInventingSourceTimes(t *testing.T) {
	doc := fixtureDocument()
	result, err := FromReviewed(encodeDocument(t, doc))
	if err != nil || result.Count != 1 {
		t.Fatal(result, err)
	}
	for _, want := range []string{"范围：当前筛选 · 1 项", "隧道与直连范围 · 无法确认", "仅服务状态", "当前观察范围", "读取完成：未提供；不补造来源时间", "读取方：网络", "配置不等于实际效果。"} {
		if !strings.Contains(result.Content, want) {
			t.Fatal("missing reviewed field", want)
		}
	}
	for _, excluded := range []string{"观察身份：", "观察平台：", "当前系统时区", "当前用户地区格式"} {
		if strings.Contains(result.Content, excluded) {
			t.Fatal("added unselected information", excluded)
		}
	}
	if strings.Count(result.Content, doc.GeneratedAt) != 1 {
		t.Fatal("report time reused as source evidence time")
	}
}

func TestTextRetainsOnlyExplicitlyIncludedEnvironmentAndObserver(t *testing.T) {
	doc := fixtureDocument()
	observer, platform, elevated, helper := "已审观察身份", "windows", false, true
	doc.Observer, doc.Platform, doc.ObserverElevated, doc.HelperAuthorized = &observer, &platform, &elevated, &helper
	doc.Scope = "all"
	doc.Findings = []detection.Finding{{ID: "timezone", Title: "当前系统时区", Status: "observed", Summary: "已显式选择的时区内容", Scope: "当前观察进程", Source: "facts", DetectedAt: "2026-10-27T01:00:01Z"}, {ID: "owned-browser-session", Title: "受控浏览器会话", Status: "observed", Summary: "仅本应用持有的有限观察", Scope: "不代表其他浏览器", Source: "browser-session", DetectedAt: "2026-10-27T01:00:02Z"}}
	result, err := FromReviewed(encodeDocument(t, doc))
	if err != nil || result.Count != 2 || !strings.Contains(result.Content, observer) || !strings.Contains(result.Content, "已显式选择的时区内容") || !strings.Contains(result.Content, "本应用受控浏览器会话") || !strings.Contains(result.Content, "不代表其他浏览器") {
		t.Fatal(result, err)
	}
}

func TestTextRejectsMalformedHiddenOrOversizedPreview(t *testing.T) {
	valid := encodeDocument(t, fixtureDocument())
	cases := map[string][]byte{
		"empty":            nil,
		"invalid-utf8":     {0xff},
		"unknown-field":    append([]byte(`{"hiddenSecret":"canary",`), valid[1:]...),
		"duplicate":        append([]byte(`{"scope":"all",`), valid[1:]...),
		"duplicate-alias":  append([]byte(`{"Scope":"all",`), valid[1:]...),
		"trailing":         append(append([]byte(nil), valid...), []byte(`{}`)...),
		"over-input-bound": []byte(strings.Repeat(" ", maxInput+1)),
	}
	for name, mutation := range map[string]func(*document){
		"schema":            func(d *document) { d.Schema = "other" },
		"scope":             func(d *document) { d.Scope = "unreviewed" },
		"generation-time":   func(d *document) { d.GeneratedAt = "not-a-time" },
		"source-time":       func(d *document) { d.Findings[0].DetectedAt = "not-a-time" },
		"status":            func(d *document) { d.Findings[0].Status = "protected" },
		"empty-title":       func(d *document) { d.Findings[0].Title = "" },
		"duplicate-finding": func(d *document) { d.Findings = append(d.Findings, d.Findings[0]) },
		"partial-observer":  func(d *document) { value := "observer"; d.Observer = &value },
		"negative-count":    func(d *document) { value := -1; d.Findings[0].Count = &value },
		"too-many-findings": func(d *document) { d.Findings = make([]detection.Finding, maxFindings+1) },
		"too-many-warnings": func(d *document) { d.Warnings = make([]string, 65) },
		"line-bound":        func(d *document) { d.Findings[0].Summary = strings.Repeat("x", 17000) },
	} {
		doc := fixtureDocument()
		mutation(&doc)
		cases[name] = encodeDocument(t, doc)
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if result, err := FromReviewed(data); err != ErrPreview || result.Content != "" || result.SHA256 != "" {
				t.Fatal("invalid preview produced copyable text", result, err)
			}
		})
	}
}

func TestTextDigestBindsExactVisibleBytesAndPreservesFindingOrder(t *testing.T) {
	doc := fixtureDocument()
	doc.Findings = append(doc.Findings, detection.Finding{ID: "second", Title: "第二项", Status: "possible", Summary: "仍可能读取", Scope: "所选来源"})
	first, err := FromReviewed(encodeDocument(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	second, err := FromReviewed(encodeDocument(t, doc))
	digest := sha256.Sum256([]byte(first.Content))
	if err != nil || first != second || first.SHA256 != hex.EncodeToString(digest[:]) || strings.Index(first.Content, "1. 隧道") >= strings.Index(first.Content, "2. 第二项") {
		t.Fatal("text bytes, digest or reviewed order changed")
	}
}

func TestTextControlCharactersCannotCreateHiddenPasteSequences(t *testing.T) {
	doc := fixtureDocument()
	doc.Findings[0].Summary = "alpha\r\n\x1b[31m\u202ebeta\tend"
	result, err := FromReviewed(encodeDocument(t, doc))
	if err != nil || strings.ContainsAny(result.Content, "\r\x1b\u202e\t") || !strings.Contains(result.Content, "结果：alpha [31m beta end\n") {
		t.Fatal(result, err)
	}
}

func TestTextOutputBoundRejectsInsteadOfTruncatingLimits(t *testing.T) {
	doc := fixtureDocument()
	doc.Findings = nil
	for index := 0; index < 100; index++ {
		doc.Findings = append(doc.Findings, detection.Finding{ID: fmt.Sprintf("f-%d", index), Title: "范围", Status: "unknown", Summary: strings.Repeat("x", 4000), Scope: "必须保留的限制"})
	}
	if result, err := FromReviewed(encodeDocument(t, doc)); err != ErrPreview || result.Content != "" {
		t.Fatal("truncated a large report", result, err)
	}
}
