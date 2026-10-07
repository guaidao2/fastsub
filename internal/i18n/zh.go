package i18n

// zh is the Simplified Chinese catalogue. Chinese is always an explicit choice:
// fastsub never switches language because of the host locale.
var zh = map[string]string{
	// Identity.
	"app.tagline": "面向渗透测试的子域名枚举与存活探测",
	"app.intro": "fastsub 把一个域名展开成一份存活主机清单：先问被动源，可选按你的字典爆破，然后解析、\n" +
		"剔除泛解析，最后对活下来的主机做 HTTP/HTTPS 探活。结果以纯文本或 JSONL 写到 stdout，\n" +
		"下一环可以直接读：argus、crackweb，任何接受主机清单的工具。",

	// Sections.
	"group.target":   "目标",
	"group.sources":  "被动源",
	"group.enum":     "主动枚举",
	"group.resolve":  "解析",
	"group.verify":   "存活探测",
	"group.output":   "输出",
	"group.misc":     "其他",
	"group.examples": "示例",

	// Target.
	"flag.domain":       "要枚举的根域名（可重复）",
	"flag.domain-list":  "从文件读要枚举的根域名（- 为 stdin），一行一个，# 为注释",
	"flag.exclude":      "要排除的主机；*.example.com 排除其子域，.example.com 连域名本身一起排除",
	"flag.exclude-file": "从文件读要排除的主机",

	// Passive sources.
	"flag.sources":         "要查询的源，逗号分隔，或 all（默认全部）",
	"flag.exclude-sources": "要跳过的源，逗号分隔",
	"flag.provider-config": "subfinder 的 provider-config.yaml 格式 API 密钥（已预留：当前内置源都不需要 key）",
	"flag.list-sources":    "列出可用的源并退出",
	"flag.only-passive":    "不解析也不探测，只报告源给出的结果",

	// Active enumeration.
	"flag.wordlist":  "爆破用的字典（可重复）",
	"flag.brute":     "按字典做爆破",
	"flag.mutate":    "对已发现的名字做变异推导",
	"flag.recursive": "对发现的子域递归展开",
	"flag.depth":     "最大递归深度（默认 1）",

	// Resolution.
	"flag.resolvers":          "从文件读 DNS 解析器",
	"flag.doh":                "通过该 DoH 端点解析（可重复）",
	"flag.no-wildcard-filter": "保留泛解析产生的应答",
	"flag.concurrency":        "并发 DNS 查询数（默认 100）",
	"flag.resolve-timeout":    "单次 DNS 查询超时（默认 3s）",

	// Liveness.
	"flag.ports":         "要探测的 HTTP 端口（默认 80,443）",
	"flag.probe":         "对解析成功的主机做 HTTP/HTTPS 探测",
	"flag.probe-timeout": "单次探测请求超时（默认 10s）",
	"flag.no-title":      "探测时不读取页面标题",
	"flag.no-favicon":    "探测时不计算 favicon 指纹",
	"flag.cert-san":      "把证书 SAN 反哺回枚举",
	"flag.no-redirect":   "探测时不跟随跳转",

	// Output.
	"flag.output":        "除 stdout 外再写一份到该文件",
	"flag.format":        "输出格式：text、json、jsonl、csv、url（默认 text）",
	"flag.output-normal": "把 nmap 风格纯文本写到该文件",
	"flag.output-json":   "把 JSON 写到该文件",
	"flag.output-all":    "以该文件名为主干一次写出全部格式",
	"flag.jsonl":         "每行输出一个 JSON 对象",
	"flag.baseline":      "只报告该历史快照里没有的内容",
	"flag.silent":        "只输出结果，不显示进度与状态",
	"flag.verbose":       "在 stderr 上给出更多细节",

	// Misc.
	"flag.lang":       "输出语言：en（默认）或 zh",
	"flag.list-langs": "列出可用的输出语言并退出",
	"flag.no-color":   "关闭颜色（fastsub 目前不带颜色输出；NO_COLOR 同样遵循）",
	"flag.help":       "打印本帮助并退出",
	"flag.version":    "打印版本信息并退出",

	// Help scaffolding.
	"help.positional_args": "要枚举的根域名，例如 example.com",
	"help.examples": "  fastsub -d example.com\n" +
		"  fastsub -d example.com --brute -w words.txt --probe\n" +
		"  fastsub -dL domains.txt -oA out/all                  # 批量枚举域名清单\n" +
		"  fastsub -d example.com | argus -iL -                 # 把主机清单交给 argus\n" +
		"  fastsub -d example.com -f url | httpx -silent        # 把 URL 交给 httpx\n" +
		"  fastsub -d example.com --baseline yesterday.jsonl    # 只报新增",
	"help.usage":           "用法：%s [选项] <域名 ...>",
	"help.run_help":        "运行 'fastsub %s --help' 查看更多信息。",
	"help.available_langs": "可用语言：%s",
	"help.author":          "作者：%s",
	"help.license":         "许可：%s",
	"help.legal":           "法律提示：fastsub 只能用于你本人拥有、或已获得明确书面授权的系统。",

	// Errors.
	"error.unknown_flag":      "未知选项 %q",
	"error.bad_bool":          "选项 %s 需要 true 或 false，收到 %q",
	"error.missing_value":     "选项 %s 需要一个值",
	"error.invalid_lang":      "未知语言 %q；可用：%s",
	"error.no_target":         "没有给定目标；请用 -d <域名>、直接给域名参数，或 -iL <文件>",
	"error.bad_format":        "未知输出格式 %q；可用 text、json、jsonl、csv 或 url",
	"error.bad_int":           "选项 %s 需要整数，收到 %q",
	"error.bad_duration":      "选项 %s 需要 5s 或 500ms 这样的时长，收到 %q",
	"error.bad_ports":         "无法解析端口列表 %q",
	"error.usage_hint":        "运行 'fastsub --help' 查看用法",
	"error.interrupted":       "已中断",
	"error.not_implemented":   "%s 已经声明但还没接线，当前构建到此为止",
	"error.brute_needs_words": "--brute 需要一个字典；请用 -w <文件>",
	"error.unknown_ports":     "%s",

	// Status lines (stderr).
	"log.nothing_found":         "%s：所有源都没有报出任何名字",
	"log.sources_done":          "%s：各源共报出 %d 个名字",
	"log.source_done":           "  %s：%d 个",
	"log.source_failed":         "  %s 失败：%v",
	"log.resolved":              "%d/%d 个名字解析成功",
	"log.unresolved":            "  %s 无法解析",
	"log.lookup_failed":         "  %s：查询失败：%v",
	"log.wildcard":              "%s 对不存在的名字也作应答（%d 个地址），这类应答会标记为泛解析",
	"log.wildcard_check_failed": "%s：泛解析检测失败：%v",
	"log.wildcard_dropped":      "已丢弃 %d 个只由泛解析产生的名字",
	"log.brute_candidates":      "字典产生 %d 个新名字",
	"log.mutate_candidates":     "变异产生 %d 个新名字",
	"log.probed":                "%d/%d 个主机在 HTTP/HTTPS 上有应答",
	"log.excluded":              "已按 --exclude 排除 %d 个名字",
	"log.baseline":              "%d 个名字已在基线中",
	"log.baseline_loaded":       "基线载入 %d 个名字",
	"log.recursing":             "递归进入 %s（第 %d 层）",
	"log.recursion_capped":      "发现 %d 个上层域名，只递归前 %d 个",
	"log.cert_names":            "有 %d 个名字来自证书",
}
