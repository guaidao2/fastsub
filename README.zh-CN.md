# fastsub

维护者：**guaidao2 & coolmoon**

[English](README.md) · **简体中文**

> 子域名枚举，最后回答一句"那里到底有没有东西"——把清单递给你链路上的下一个工具。

fastsub 是用 Go 写的命令行工具。它会向被动源要一个域名下的名字，可选地按你自己的
字典造出更多，然后解析、剔除泛解析产生的应答，最后对活下来的做 HTTP/HTTPS 探活。结果以
纯文本或带版本的 JSONL 写到 stdout，下一环——[argus](https://github.com/guaidao2/argus)、
[crackweb](https://github.com/guaidao2/crackweb)，任何读主机清单的工具——拿来就能用，
不需要中间层做转换。

单一静态二进制，无运行时依赖，无数据库，无服务端。MIT 开源。默认英文，可显式切换简体中文。

---

## 为什么还要再造一个子域工具

因为真正有用的部分不是枚举本身，而是枚举接进了什么。

- **是一份约定，不只是输出。** `-dL` 读的文件格式与 argus 的 `-iL` 完全一致（`#` 注释、
  `-` 表示 stdin），`-f jsonl` 每台主机吐一条带版本、语言中立的记录。两个工具认这套约定，
  中间就不需要胶水脚本。
- **泛解析检测不是可选项。** 一个开了 `*.example.com` 的zone，会让字典里每一个词都"解析成功"。
  fastsub 在判断任何名字之前先测这一条，并把字典造出来的东西如实地标成它本来的样子。
- **猜测会被标明是猜测。** 每条记录上的 `sources` 字段写着这个名字从哪来——`crtsh`、
  `urlscan`、`brute`、`mutate`、`certificate`。字典造出来的名字和证书签发过的名字不是一回
  事，读的人不必自己猜手上这条是哪种。
- **不做平台。** 没有数据库、没有 Web 界面、没有常驻进程。会调度扫描的资产系统该是另一个
  程序；fastsub 的职责是做一个接口稳定的命令，供它调用。

## 安装

```sh
# 从源码安装（需要 Go 1.24+）
go install github.com/guaidao2/fastsub/cmd/fastsub@latest

# 或构建静态二进制
make build            # -> bin/fastsub

# 交叉编译全部支持平台到 ./dist
make cross
```

## 快速开始

```sh
# 这个域名上，公开数据里有什么
fastsub -d example.com

# ……以及它们有没有应答
fastsub -d example.com --probe

# ……再按字典造一批名字，一并检查
fastsub -d example.com --brute -w words.txt --probe

# 把主机清单交给 argus
fastsub -d example.com | argus -iL - -sV --findings

# 把 URL 交给 crackweb，或者交给 httpx
fastsub -d example.com -f url | httpx -silent

# 只报告昨天还没有的
fastsub -d example.com --baseline yesterday.jsonl
```

## 语言

无论系统 locale 是什么，默认输出语言都是英文。中文始终是一个**显式**选择：

```sh
fastsub --help                   # 英文
fastsub --lang zh --help         # 中文
FASTSUB_LANG=zh fastsub --help   # 通过环境变量达到同样效果
```

机器可读输出保持语言中立：字段名和取值都不受 `--lang` 影响，因为解析器不该去猜操作者
读哪种语言。

## 流水线

```
  被动源 ───────────┐
                    ├──▶ 名字 ──▶ 解析 ──▶ 泛解析过滤 ──▶ 探活 ──▶ 输出
  字典 ─────────────┤              │                        │
  变异 ─────────────┘              └── 证书 SAN ◀────────────┘
```

每个阶段都能跳过。`--only-passive` 停在第一步；不给 `--probe`，就只按 DNS 应答报告名字。

## 目标

`-d`、直接写在后面的参数、`-dL` 三者都表示**要枚举的根域名**：会去问被动源、会应用字典、
会向下递归。`-dL` 是 `-d` 的批量形式，用于清单在文件里、或者从管道过来的场合。

```sh
fastsub -d example.com                      # 一个域名
fastsub -d a.com -d b.com                   # 多个，可重复
fastsub example.com                         # 直接作为参数也行
fastsub -dL domains.txt                     # 清单，一行一个，# 为注释
cat domains.txt | fastsub -dL -             # 或者从管道读
fastsub -d example.com --exclude '*.dev.example.com'
```

`--exclude` 有三种写法，含义各不相同：

| 写法 | 排除范围 |
| --- | --- |
| `mail.example.com` | 仅这一台主机 |
| `*.example.com` | 所有子域，不含域名本身 |
| `.example.com` | 该域名及其下所有名字 |

## 被动源

当前构建内置七个源，都不需要 API key：`crtsh`、`certspotter`、`subdomaincenter`、
`urlscan`、`rapiddns`、`hackertarget`、`otx`。

```sh
fastsub --list-sources
fastsub -d example.com -s crtsh,certspotter
fastsub -d example.com --exclude-sources otx,hackertarget
```

它们并不一样健康，fastsub 把这当作常态而不是故障。crt.sh 忙的时候回 502（它拿到三次
重试机会）；hackertarget 的免费额度当天会用尽；otx 限流很凶。某个源失败只会在 stderr 上
报一行，整轮继续——一个端点是死的，不是返回空结果的理由。每个源有独立的超时，所以一个
慢源只花掉它自己的预算，不拖住整轮。

源从不接触目标本身。这里读的全是公开数据集，也正是被动枚举安静的原因。

## 主动枚举

```sh
fastsub -d example.com --brute -w words.txt     # 字典里的词逐个拼上去
fastsub -d example.com --mutate                 # 对已发现的名字做变异
fastsub -d example.com --recursive --depth 2    # 对发现的子域继续展开
```

`--brute` 需要配 `-w`，就是字面意思；`--mutate` 从已知的标签推导出 `dev-api`、`api-dev`、
`api1` 这一类。`--recursive` 与前两者不同：像 `dev.example.com` 这样的名字自成命名空间，
它下面的一切都不会出现在 `example.com` 的证书里，也不会被针对 `example.com` 的字典命中。

猜测的量是被刻意限制的。变异最多覆盖 60 个标签，每层递归最多进入 25 个上层域名。一轮
生成一千万个候选不叫有产出，那叫对着你指定的解析器泼水。

每个猜测在输出里都带着来源标记（`brute`、`mutate`），因为字典造出来的名字和证书签发过的
名字，不是同一种东西。

## 解析与泛解析

```sh
fastsub -d example.com -c 200                      # 并发查询数
fastsub -d example.com -rl resolvers.txt           # 用自己的解析器
fastsub -d example.com --doh https://dns.alidns.com/dns-query
fastsub -d example.com --no-wildcard-filter        # 保留泛解析应答
```

**泛解析检测跑在判断任何名字之前。** fastsub 会向这个 zone 随机问三个不可能存在的标签。
三个都解析成功，说明该 zone 对一切名字都作应答，那么字典里每一个词也都会"解析成功"。
地址与泛解析重合的应答会被标记为泛来源，默认不进报告。这不是讲究：少了这一步，对一个
开了泛解析的 zone 做爆破，会产出成千上万条看着真实、实际不存在的结果。

这个判定刻意保守——三个探针必须全部解析成功才算——因为误判一个泛解析，代价是每一台恰好
与它共用地址的真实主机。如果你就是想要这些名字，`--no-wildcard-filter` 会保留它们，
并带着标记。

解析器池优先使用一直在应答的那个，并主动放弃不行的那个：从你的网络到不了的服务器只花掉
一次超时，而不是每个名字都花一次。`--doh` 用你点名的端点取代整个池子——当 UDP 53 被
过滤或被改写时，这正是你要的。

## 存活探测

```sh
fastsub -d example.com --probe
fastsub -d example.com --probe -p 80,443,8080,8443
fastsub -d example.com --probe --no-title
fastsub -d example.com --cert-san            # 把证书 SAN 反哺回枚举
```

DNS 应答只能说明名字存在；只有响应才能说明那里有东西在服务。`--probe` 按你指定的端口
（默认 80 和 443）分别试 HTTP 与 HTTPS，记录状态码、页面标题、`Server` 头，以及跳转最终
落到了哪里。

它还会给 `/favicon.ico` 算指纹：图标的 MurmurHash3，base64 包装方式与 Shodan、FOFA 完全
一致，所以这个数字可以直接和它们公布的数据对照。图标属于应用而不属于主机——两个地址不同、
证书不同、标题也不同的名字，在同一套产品服务时会有同一个数字，而这种关联关系不在任何一条
DNS 记录里。`--no-favicon` 与 `--no-title` 可以关掉这两个额外请求。

403、404 也算活着。这一步问的是"那里有没有东西"，不是"它对不对你好"。只有传输层失败
才算死。

`--cert-san` 读主机呈现的证书上的 subject alternative names，把其中新的那些解析进来。
证书是"这个域名被签发过"的证据，包括任何被动源都没见过的名字；这一轮只跑一次，所以
一个新名字带来一张新证书，不会让枚举一路走出你的域名之外。

## 输出

```sh
fastsub -d example.com                        # text：一行一个裸主机名
fastsub -d example.com -f url                 # 一行一个 URL（会开探活）
fastsub -d example.com -f jsonl               # 一行一条 JSON 记录
fastsub -d example.com -f json                # 一份完整文档
fastsub -d example.com -f csv                 # 一行一个 endpoint
fastsub -d example.com -o out.txt             # 同时写一份到文件
fastsub -d example.com -oA out/example        # 一次写出 .txt/.jsonl/.csv
```

结果只走 stdout，进度只走 stderr，所以 `fastsub ... | jq` 是安全的，`--silent` 只关掉
进度、不动数据。`-oN`、`-oJ`、`-oA` 的拼写与 argus 保持一致。

### JSONL 契约

每台主机一条记录，最后一行是汇总：

```json
{"type":"host","host":"api.example.com","sources":["crtsh","urlscan"],"ips":["203.0.113.10"],"cname":"lb.example.net","alive":true,"urls":[{"url":"https://api.example.com/","scheme":"https","port":443,"status_code":200,"title":"API","server":"nginx","content_length":1024,"favicon_hash":-1588080585}]}
{"type":"summary","schema":"fastsub/v1","hosts":1240,"alive":18,"elapsed_seconds":42.1}
```

字段只会增加，不会改名。`schema` 标明版本，使用者可以拒绝一个它看不懂的版本，而不是
去猜字段含义。

**没测过的字段不会出现**，这是刻意的：没跑探活的运行里没有 `alive`，而不是写成 `false`
——"没人看过"和"看过，答案是否"是两种不同的断言。`-f csv` 遵循同一条规则，对应单元格
留空。

### 只报告新增

```sh
fastsub -d example.com -f jsonl > snapshots/2026-10-07.jsonl
fastsub -d example.com --baseline snapshots/2026-10-07.jsonl
```

`--baseline` 读入上一次的快照，在解析任何名字之前就把已在其中的丢掉，所以定时任务只为
新增付出查询成本。快照可以是 fastsub 自己的任何一种格式——JSONL、JSON、CSV、纯文本，
甚至 `-v` 的文本输出——因为格式是按内容判断的，不是按文件名。这里没有数据库：快照就是
一个文件。

定时任务有一个注意点：有些源并不稳定。`subdomaincenter` 对同一个域名每次返回的集合都不
一样，拿它做对比会把数据集的抖动报成新增。如果你要的是关于目标的变更，就把它排除掉
（`--exclude-sources subdomaincenter`）。

## 与其它工具联动

这一族工具刻意共享参数顺序与文件格式，于是它们只用管道就能组合：

```sh
# 先枚举，再扫端口
fastsub -d example.com | argus -iL - -sV --findings

# 先枚举，再扫 Web 面
fastsub -d example.com -f url | httpx -silent

# 只留下有应答的 endpoint，交给扫描器
fastsub -d example.com --probe -f url | sort -u > urls.txt
```

在 Windows 上走管道前有一件事要知道：如果 PowerShell 的输出编码设成默认的
`Encoding.UTF8`，它会往管道里写一个 UTF-8 BOM，下游工具会把 BOM 当成第一个主机名的
一部分。纯 ASCII 主机清单场景干脆别设输出编码，或者用
`[System.Text.UTF8Encoding]::new($false)`。

## 设计取舍

**误报是最大的敌人。** 一个什么都报的工具会训练用户忽略它。三道防线：泛解析检测；
"只有解析成功才算存在"这条规则；以及每个名字都带着来源标注。

**失败要看得见。** 源失败了、名字解析不出来、解析器不再应答——全部计数并上报。一次
"没有结果"的运行和一次"根本跑不起来"的运行，不能长得一样。

**猜测量有上限。** 字典、变异、递归都封顶，因为另一个选择是：一个对着目标解析器泼水、
然后告诉你什么都没找到的工具。

**默认只做被动。** 解析是 fastsub 第一件目标能看见的事。`--only-passive` 停在它之前；
fastsub 里也没有任何地方把字典当成证据。

## 项目结构

```
cmd/fastsub/          可执行入口
internal/
  baseline/           历史快照，以及"新增"的定义
  cli/                参数解析、命令分发、双语帮助
  enum/               字典与变异候选
  i18n/               中英文案表
  model/              全流程共享的数据形状与 JSONL schema
  output/             text、JSON、JSONL、CSV、URL 渲染
  pipeline/           一轮运行本身：源、解析、探活、输出
  resolve/            解析器池、DoH、泛解析检测
  scope/              一次运行允许报告什么
  source/             被动源，一源一文件
  verify/             HTTP/HTTPS 存活探测
  version/            版本与署名
```

## 开发

```sh
make            # fmt、vet、test、build
make check      # gofmt -l、go vet、go test
make cross      # 生成 linux、darwin、windows 静态二进制
```

没有 make 时，真正要紧的就两条命令：`go build -trimpath -o bin/fastsub ./cmd/fastsub`
和 `go vet ./... && go test ./...`。发布构建时注入版本：
`-ldflags "-X github.com/guaidao2/fastsub/internal/version.Version=1.0.0"`。

有两条约定值得说明。所有面向用户的文案都在 `internal/i18n` 里，翻译缺一条、或者中文里的
printf 占位符与英文对不上，测试都会让构建失败。另外每个阶段都通过接口拿到它需要的东西——
`pipeline.Resolver`、`pipeline.Sink`、`source.Source`——这正是整条链路能在不联网的情况下
被完整测试的原因。

## 免责声明

fastsub 仅供获得合法授权的安全测试、CTF 比赛与教学研究使用。请勿用于你既不拥有、
也未获得书面授权的系统。未经授权枚举他人系统的子域名，在多数司法辖区可能构成违法。

## 许可

MIT —— 见 [LICENSE](LICENSE)。Copyright (c) 2026 guaidao2 & coolmoon。
