package detection

import "fmt"

// Settings evidence says what the service reports configuring. It does not
// inspect resolver traffic, read logs/credentials, or test every file ACL.
func addStorageEvidence(r *Report, n Network) {
	dns := Finding{ID: "network-dns", Title: "DNS 配置范围", Category: "网络路径", Audience: "network", Status: "unknown", Summary: "没有可用的 DNS 配置摘要；解析路径未知。", Evidence: "仅接收已认证服务的设置类别，不读取或导出解析器地址", Scope: "配置证据；没有抓包、发起 DNS 查询或验证其他应用的解析路径"}
	if n.Available && n.SettingsAvailable && n.RemoteDNSMode == "doh" && (n.LocalDNSMode == "system" || n.LocalDNSMode == "doh") {
		dns.Summary = "远程解析配置为 DoH；本地解析配置为 DoH。"
		if n.LocalDNSMode == "system" {
			dns.Summary = "远程解析配置为 DoH；本地解析设置为 system。严格全局模式可能由构建器替换，实际路径未核验。"
		}
		dns.Summary += " 配置不证明无 DNS 泄露或所有应用都使用此解析器。"
	}
	logs := Finding{ID: "local-logs", Title: "日志级别与保留", Category: "本机数据", Audience: "app", Status: "unknown", Summary: "没有可用的日志配置摘要；保留范围未知。", Evidence: "只读设置摘要；未读取日志内容", Scope: "服务日志配置；不证明旧文件已删除、所有日志均受此设置覆盖或不可恢复"}
	level := map[string]string{"debug": "调试", "info": "信息", "warn": "警告", "error": "错误"}[n.LogLevel]
	if n.Available && n.SettingsAvailable && level != "" && n.LogDays != nil && *n.LogDays >= 0 && *n.LogDays <= 365 {
		retention := fmt.Sprintf("%d 天", *n.LogDays)
		if *n.LogDays == 0 {
			retention = "不按天数自动清理"
		}
		logs.Summary = fmt.Sprintf("日志级别配置为%s；保留配置：%s。配置不等于已清除旧记录。", level, retention)
	}
	credentials := Finding{ID: "credentials-storage", Title: "订阅与凭据存储", Category: "本机数据", Audience: "app", Status: "possible", Summary: "尚未实现 DPAPI 凭据加密；订阅和缓存可能包含敏感信息。文件权限不等于加密，也不能隔离管理员。", Evidence: "实现能力边界；本次未读取订阅、密码、缓存内容或枚举全部文件权限", Scope: "本客户端存储；不代表浏览器或其他应用的凭据保护"}
	r.Findings = append(r.Findings, dns, logs, credentials)
}
