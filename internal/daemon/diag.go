package daemon

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// 诊断包:日志、脱敏后的配置与设置、路由表、网卡信息。凭据类字段一律打码,订阅地址只留主机名。
var secretKeys = map[string]bool{"password": true, "uuid": true, "privatekey": true, "psk": true, "presharedkey": true, "secret": true, "token": true, "credential": true, "credentials": true, "auth": true, "authstr": true, "authorization": true, "proxyauthorization": true, "apikey": true}

func secretKey(key string) bool {
	key = strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	return secretKeys[key] || strings.HasSuffix(key, "password") || strings.HasSuffix(key, "token") || strings.HasSuffix(key, "secret")
}

func redact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if secretKey(k) {
				out[k] = "***"
			} else if strings.HasSuffix(strings.ToLower(k), "url") {
				if s, ok := val.(string); ok {
					out[k] = redactURL(s)
				} else {
					out[k] = redact(val)
				}
			} else {
				out[k] = redact(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = redact(x[i])
		}
		return out
	case string:
		return redactText(x)
	default:
		return v
	}
}

var (
	diagURL       = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s<>"'\x60]+`)
	subPath       = regexp.MustCompile(`(?i)(/sub/)[^\s/?#"'<>]+`)
	bearerToken   = regexp.MustCompile(`(?i)\bBearer[ \t]+[^\s,"'<>]+`)
	logCredential = regexp.MustCompile(`(?i)\b((?:access[_-]?|refresh[_-]?)?token|(?:web[_-]?|obfs[_-]?|proxy[_-]?)?password|client[_-]?secret|private[_-]?key|api[_-]?key|authorization|uuid|psk)("?[ \t]*[:=][ \t]*)("[^"\r\n]*"|'[^'\r\n]*'|[^\s,;"']+)`)
)

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		if raw == "" {
			return ""
		}
		return "***" // malformed or opaque subscription links must not echo credentials
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ws", "wss", "ftp", "socks", "socks5", "trojan", "ss", "vless", "hysteria", "hysteria2", "tuic":
		return (&url.URL{Scheme: strings.ToLower(u.Scheme), Host: u.Host}).String()
	default:
		// vmess and other opaque URI schemes can encode the entire credential
		// into what url.Parse considers a hostname.
		return "***"
	}
}

func redactText(s string) string {
	// Logs may include JSON-escaped URLs. Normalization only affects the
	// exported copy, never the underlying log or settings file.
	s = strings.ReplaceAll(s, `\/`, `/`)
	s = diagURL.ReplaceAllStringFunc(s, redactURL)
	s = bearerToken.ReplaceAllString(s, "Bearer ***")
	s = logCredential.ReplaceAllStringFunc(s, redactCredential)
	return subPath.ReplaceAllString(s, "${1}***")
}

// redactCredential 把 key: value 里的 value 换成 ***,**引号保留**。
// info.json / config.redacted.json 是先序列化再整段过这个正则的;m29 的写法连闭合引号一起吃掉,
// 于是 "psk=xxx" 这种出现在节点名里的值会让整个 JSON 变成非法的,诊断包里最重要的两个文件打不开。
func redactCredential(m string) string {
	sub := logCredential.FindStringSubmatch(m)
	if len(sub) < 4 {
		return m
	}
	key, sep, val := sub[1], sub[2], sub[3]
	if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
		return key + sep + string(val[0]) + "***" + string(val[0])
	}
	return key + sep + "***"
}

// crashExcerpt 崩溃记录摘要。每段(以"== … 启动"抬头分开)的原因行在最前面("fatal error: …"、"Exception 0x…"、
// "panic: …"),后面跟着全部 goroutine 的栈,动辄上千行;只取整个文件的最后几百行会把原因截掉。
// 只挑有内容的段(抬头以外还有非空行),取最后 3 段,每段超过 200 行就留头 150 行、尾 50 行:每次正常启动都会写
// 一行抬头,按段数取的话,崩溃之后服务被自动拉起、再开两次机,崩溃那段就被只有抬头的段挤出去了。
// 最后一段有内容的之后还有几次启动,记一行。没有抬头的(Android 那份)照旧取最后 300 行。
func crashExcerpt(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return ""
	}
	return crashTextExcerpt(string(b))
}

func crashTextExcerpt(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var starts []int
	for i, l := range lines {
		if strings.HasPrefix(l, "== ") {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		if len(lines) > 300 {
			lines = lines[len(lines)-300:]
		}
		return strings.Join(lines, "\n")
	}
	if starts[0] > 0 {
		starts = append([]int{0}, starts...) // 第一个抬头之前的内容(轮转时被截在中间的那段)单算一段
	}
	type seg struct{ s, e int }
	var withContent []seg
	lastContent := -1
	for k, st := range starts {
		end := len(lines)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		for _, l := range lines[st:end] {
			if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "== ") {
				withContent = append(withContent, seg{st, end})
				lastContent = k
				break
			}
		}
	}
	if len(withContent) == 0 {
		// 全是启动抬头:没有崩溃记录。留最后一行抬头,看得出记录本身是开着的
		return lines[starts[len(starts)-1]] + "\n(没有崩溃记录)"
	}
	if len(withContent) > 3 {
		withContent = withContent[len(withContent)-3:]
	}
	var out []string
	for _, g := range withContent {
		blk := lines[g.s:g.e]
		if len(blk) > 200 {
			out = append(out, blk[:150]...)
			out = append(out, fmt.Sprintf("…(省略 %d 行)…", len(blk)-200))
			out = append(out, blk[len(blk)-50:]...)
		} else {
			out = append(out, blk...)
		}
	}
	if later := len(starts) - 1 - lastContent; later > 0 {
		out = append(out, fmt.Sprintf("…(之后又启动了 %d 次,没有崩溃记录;最后一次:%s)", later, lines[starts[len(starts)-1]]))
	}
	return strings.Join(out, "\n")
}
