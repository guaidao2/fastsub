# fastsub

Maintained by **guaidao2 & coolmoon**

**English** · [简体中文](README.zh-CN.md)

> Subdomain enumeration that ends with an answer to "is anything there?" —
> built to hand its list to the next tool in the chain.

fastsub is a command-line tool written in Go. It asks passive sources for names
under a domain, optionally invents more from a wordlist, resolves what it finds,
throws away the answers a wildcard record produced, and probes the survivors
over HTTP and HTTPS. Results go to stdout as plain text or versioned JSONL, so
the next link — [argus](https://github.com/guaidao2/argus),
[crackweb](https://github.com/guaidao2/crackweb), anything that reads a host
list — can consume them without a translation layer.

One static binary, no runtime dependencies, no database, no server. MIT
licensed. English by default, Simplified Chinese on request.

---

## Why another subdomain tool

Because the useful part is not the enumeration, it is what the enumeration
plugs into.

- **A contract, not just output.** `-dL` reads the same file format argus's
  `-iL` reads (`#` comments, `-` for stdin), and `-f jsonl` emits a versioned,
  language neutral record per host. Two tools that agree on that need no glue
  script.
- **The wildcard check is not optional.** A zone that answers `*.example.com`
  makes every wordlist entry resolve. fastsub tests for it before it judges any
  name, and reports what a wordlist invented as what it is.
- **A guess is labelled as a guess.** A name from a certificate, a name from a
  wordlist and a name from a crawl are not the same evidence, and the output
  says which is which.
- **No platform.** No database, no web UI, no daemon. An asset system that
  schedules scans belongs in its own program; fastsub's job is to be a command
  with a stable interface for it to call.

## Install

```sh
# from source (Go 1.24+)
go install github.com/guaidao2/fastsub/cmd/fastsub@latest

# or build the static binary
make build            # -> bin/fastsub

# every supported platform into ./dist
make cross
```

## Quick start

```sh
# what have people published about this domain
fastsub -d example.com

# ...and does anything answer
fastsub -d example.com --probe

# ...and invent more names, then check those too
fastsub -d example.com --brute -w words.txt --probe

# hand the host list to argus
fastsub -d example.com | argus -iL - -sV --findings

# hand the URLs to crackweb, or to httpx
fastsub -d example.com -f url | httpx -silent

# report only what was not there yesterday
fastsub -d example.com --baseline yesterday.jsonl
```

## Language

English is the default output language, whatever the host locale is. Chinese is
always an explicit choice:

```sh
fastsub --help                   # English
fastsub --lang zh --help         # Chinese
FASTSUB_LANG=zh fastsub --help   # the same, via the environment
```

Machine-readable output is language neutral: field names and values do not
depend on `--lang`, because a parser should not have to know what language the
operator reads.

## The chain

```
  passive sources ──┐
                    ├──▶ names ──▶ resolve ──▶ wildcard filter ──▶ probe ──▶ output
  wordlist ─────────┤                │                              │
  permutations ─────┘                └── certificate SANs ◀──────────┘
```

Every stage is skippable. `--only-passive` stops after the first one; leaving
out `--probe` reports names on their DNS answer alone.

## Targets

`-d`, a bare argument and `-dL` all name **root domains to enumerate**: sources
are asked about them, the wordlist applies, and their subdomains are walked.
`-dL` is the batch form of `-d`, for when the list lives in a file or arrives on
a pipe.

```sh
fastsub -d example.com                      # one domain
fastsub -d a.com -d b.com                   # several, repeatable
fastsub example.com                         # positional works too
fastsub -dL domains.txt                     # a list, one per line, '#' comments
cat domains.txt | fastsub -dL -             # or from a pipe
fastsub -d example.com --exclude '*.dev.example.com'
```

`--exclude` takes three spellings and they mean different things:

| Pattern | Excludes |
| --- | --- |
| `mail.example.com` | that one host |
| `*.example.com` | every subdomain, not the domain itself |
| `.example.com` | the domain and everything under it |

## Passive sources

Seven sources ship with this build, none of them needing an API key:
`crtsh`, `certspotter`, `subdomaincenter`, `urlscan`, `rapiddns`,
`hackertarget`, `otx`.

```sh
fastsub --list-sources
fastsub -d example.com -s crtsh,certspotter
fastsub -d example.com --exclude-sources otx,hackertarget
```

They are not equally healthy, and fastsub treats that as normal rather than
fatal. crt.sh answers 502 when it is busy (it gets three attempts);
hackertarget's free quota runs out for the day; otx rate limits hard. A source
that fails is reported on stderr and the run continues with the others — one
dead endpoint is not a reason to return nothing. Each source has its own
timeout, so a slow one costs its own budget and not the run's.

A source never touches the target. Everything here reads public datasets, which
is what makes passive enumeration quiet.

## Active enumeration

```sh
fastsub -d example.com --brute -w words.txt     # every word against the domain
fastsub -d example.com --mutate                 # permute the labels found
fastsub -d example.com --recursive --depth 2    # walk the subdomains found
```

`--brute` needs `-w`, and it means `-w`; `--mutate` derives `dev-api`,
`api-dev`, `api1` and the rest from labels already known. `--recursive` is
different from both: a name like `dev.example.com` is a namespace of its own,
and nothing under it appears in `example.com`'s certificate or in a wordlist
applied to `example.com`.

Guesses are bounded on purpose. Permutations cover at most 60 labels, and one
level of recursion walks at most 25 parent domains. A run that generates ten
million candidates has not found anything, it has flooded whatever resolver you
pointed it at.

Every guess is labelled in the output with where it came from (`brute`,
`mutate`), because a name a wordlist invented is not the same thing as a name a
certificate was issued for.

## Resolution and the wildcard

```sh
fastsub -d example.com -c 200                      # parallel lookups
fastsub -d example.com -rl resolvers.txt           # your own resolvers
fastsub -d example.com --doh https://dns.alidns.com/dns-query
fastsub -d example.com --no-wildcard-filter        # keep wildcard answers
```

**The wildcard check runs before any name is judged.** fastsub asks the zone for
three random labels that could not exist. If all three resolve, the zone answers
for everything, and every name a wordlist invents will "resolve" too. Answers
matching the wildcard's addresses are marked as wildcard-sourced and, by
default, left out of the report. This is not a nicety: without it a brute force
against a wildcarded zone produces thousands of results that look real and are
not.

The check is deliberately conservative — all three probes must resolve — because
inventing a wildcard costs every real host that happens to share an address with
it. If you want those names anyway, `--no-wildcard-filter` keeps them, marked.

The resolver pool prefers a resolver that has been answering and gives up on one
that has not: a server that cannot be reached from your network costs one
timeout, not one per name. `--doh` replaces the pool with the endpoint you name,
which is what you want when UDP to port 53 is filtered or rewritten.

## Liveness

```sh
fastsub -d example.com --probe
fastsub -d example.com --probe -p 80,443,8080,8443
fastsub -d example.com --probe --no-title
fastsub -d example.com --cert-san            # feed certificate SANs back in
```

A DNS answer says a name exists; only a response says something is serving it.
`--probe` asks HTTP and HTTPS on the ports you name (80 and 443 by default) and
records the status code, the page title, the `Server` header and where a
redirect ended up.

It also fingerprints `/favicon.ico`: the MurmurHash3 of the icon, base64 wrapped
the way Shodan and FOFA compute it, so the number is directly comparable with
what those services publish. An icon belongs to the application rather than the
host — two names with different addresses, different certificates and different
titles carry the same number when the same product serves both, which is a
relationship no DNS record contains. `--no-favicon` and `--no-title` turn the two
extra requests off.

A 403 or a 404 counts as alive. The question here is whether anything is there,
not whether it likes the request. Only a transport failure counts as dead.

`--cert-san` reads the subject alternative names off the certificate a host
presented and resolves the ones that are new. A certificate is evidence that a
name was issued for, including names no passive source ever saw; that pass runs
once, so a new name bringing a new certificate does not walk the enumeration off
into another domain.

## Output

```sh
fastsub -d example.com                        # text: one bare host per line
fastsub -d example.com -f url                 # one URL per endpoint (probes)
fastsub -d example.com -f jsonl               # one JSON record per host
fastsub -d example.com -f json                # one document
fastsub -d example.com -f csv                 # one row per endpoint
fastsub -d example.com -o out.txt             # also write to a file
fastsub -d example.com -oA out/example        # .txt, .jsonl and .csv at once
```

Results go to stdout and progress goes to stderr, so `fastsub ... | jq` is safe
and `--silent` quiets the progress without touching the data. `-oN`, `-oJ` and
`-oA` are spelled the way argus spells them.

### The JSONL contract

One record per host, then one summary line:

```json
{"type":"host","host":"api.example.com","sources":["crtsh","urlscan"],"ips":["203.0.113.10"],"cname":"lb.example.net","alive":true,"urls":[{"url":"https://api.example.com/","scheme":"https","port":443,"status_code":200,"title":"API","server":"nginx","content_length":1024,"favicon_hash":-1588080585}]}
{"type":"summary","schema":"fastsub/v1","hosts":1240,"alive":18,"elapsed_seconds":42.1}
```

Fields are added to, never renamed. `schema` names the version so a consumer can
refuse one it does not understand instead of guessing.

### Reporting only what is new

```sh
fastsub -d example.com -f jsonl > snapshots/2026-10-07.jsonl
fastsub -d example.com --baseline snapshots/2026-10-07.jsonl
```

`--baseline` reads a previous snapshot and drops every name already in it before
anything is resolved, so a scheduled run costs queries only for what is new. The
snapshot can be any of fastsub's own formats — JSONL, JSON, CSV, a plain list,
even `-v` text output — because the format is detected from the content rather
than the file name. There is no database: a snapshot is a file.

## Working with other tools

Argument order and file formats are shared on purpose across this family of
tools, so they compose with pipes and nothing else:

```sh
# enumerate, then scan what was found
fastsub -d example.com | argus -iL - -sV --findings

# enumerate, then scan the web surface
fastsub -d example.com -f url | httpx -silent

# keep only endpoints that answered, as input for a scanner
fastsub -d example.com --probe -f url | sort -u > urls.txt
```

On Windows, one thing to know before piping: PowerShell writes a UTF-8 BOM into
a pipeline if its output encoding is set to the default `Encoding.UTF8`, and the
tool downstream reads the BOM as part of the first host name. Either leave the
encoding alone for ASCII host lists or set
`[System.Text.UTF8Encoding]::new($false)`.

## Design notes

**False positives are the enemy.** A tool that reports everything teaches its
user to ignore it. Three things guard against that: the wildcard check, the rule
that a name only counts as existing when it resolves, and labels on every name
saying where it came from.

**Failures are visible.** A source that fails, a name that does not resolve, a
resolver that stops answering — all counted and reported. A run that returned
nothing and a run that could not run must not look the same.

**Bounded guesses.** Wordlists, permutations and recursion all have a ceiling,
because the alternative is a tool that floods the target's resolvers and tells
you it found nothing.

**Only passive by default.** Resolution is the first thing fastsub does that the
target can see. `--only-passive` stops before it, and nothing in fastsub
pretends a wordlist is evidence.

## Project layout

```
cmd/fastsub/          executable entry point
internal/
  baseline/           previous snapshots, and what "new" means
  cli/                argument parsing, dispatch, bilingual help
  enum/               wordlist and permutation candidates
  i18n/               English and Chinese message catalogues
  model/              the shape every stage shares, and the JSONL schema
  output/             text, JSON, JSONL, CSV and URL rendering
  pipeline/           the run itself: sources, resolution, probing, output
  resolve/            resolver pool, DoH, wildcard detection
  scope/              what a run may report
  source/             passive sources, one file each
  verify/             HTTP and HTTPS liveness probing
  version/            version and authorship
```

## Development

```sh
make            # fmt, vet, test, build
make check      # gofmt -l, go vet, go test
make cross      # static binaries for linux, darwin and windows
```

Without make, the two commands that matter are
`go build -trimpath -o bin/fastsub ./cmd/fastsub` and
`go vet ./... && go test ./...`. A release build sets the version:
`-ldflags "-X github.com/guaidao2/fastsub/internal/version.Version=1.0.0"`.

Two conventions matter here. Every user-facing string lives in `internal/i18n`,
and the catalogue tests fail the build when a translation is missing or when its
printf verbs drift from the English original. And a stage is handed what it
needs as an interface — `pipeline.Resolver`, `pipeline.Sink`, `source.Source` —
which is what lets the whole chain be tested without a network.

## Disclaimer

fastsub is intended for authorised security testing, CTF practice and research
only. Do not use it against systems you neither own nor have written permission
to test. Enumerating subdomains of a system without authorisation may be
unlawful in your jurisdiction.

## License

MIT — see [LICENSE](LICENSE). Copyright (c) 2026 guaidao2 & coolmoon.
