<div align="center">

# 📦&nbsp;&nbsp;go-foundation

**The shared, domain-agnostic foundation kit for modern Go services**

<br/>

<a href="https://github.com/mrz1836/go-foundation/releases"><img src="https://img.shields.io/github/release-pre/mrz1836/go-foundation?include_prereleases&style=flat-square&logo=github&color=black" alt="Release"></a>
<a href="https://golang.org/"><img src="https://img.shields.io/github/go-mod/go-version/mrz1836/go-foundation?style=flat-square&logo=go&color=00ADD8" alt="Go Version"></a>
<a href="https://github.com/mrz1836/go-foundation/blob/master/LICENSE"><img src="https://img.shields.io/github/license/mrz1836/go-foundation?style=flat-square&color=blue" alt="License"></a>

<br/>

<table align="center" border="0">
  <tr>
    <td align="right">
       <code>CI / CD</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://github.com/mrz1836/go-foundation/actions"><img src="https://img.shields.io/github/actions/workflow/status/mrz1836/go-foundation/fortress.yml?branch=master&label=build&logo=github&style=flat-square" alt="Build"></a>
       <a href="https://github.com/mrz1836/go-foundation/actions"><img src="https://img.shields.io/github/last-commit/mrz1836/go-foundation?style=flat-square&logo=git&logoColor=white&label=last%20update" alt="Last Commit"></a>
    </td>
    <td align="right">
       &nbsp;&nbsp;&nbsp;&nbsp; <code>Quality</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://codecov.io/gh/mrz1836/go-foundation"><img src="https://codecov.io/gh/mrz1836/go-foundation/branch/master/graph/badge.svg?style=flat-square" alt="Coverage"></a>
    </td>
  </tr>

  <tr>
    <td align="right">
       <code>Security</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://scorecard.dev/viewer/?uri=github.com/mrz1836/go-foundation"><img src="https://api.scorecard.dev/projects/github.com/mrz1836/go-foundation/badge?style=flat-square" alt="Scorecard"></a>
       <a href=".github/SECURITY.md"><img src="https://img.shields.io/badge/policy-active-success?style=flat-square&logo=security&logoColor=white" alt="Security"></a>
    </td>
    <td align="right">
       &nbsp;&nbsp;&nbsp;&nbsp; <code>Community</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://github.com/mrz1836/go-foundation/graphs/contributors"><img src="https://img.shields.io/github/contributors/mrz1836/go-foundation?style=flat-square&color=orange" alt="Contributors"></a>
       <a href="https://mrz1818.com/"><img src="https://img.shields.io/badge/donate-bitcoin-ff9900?style=flat-square&logo=bitcoin" alt="Bitcoin"></a>
    </td>
  </tr>
</table>

</div>

<br/>
<br/>

<div align="center">

### <code>Project Navigation</code>

</div>

<table align="center">
  <tr>
    <td align="center" width="33%">
       🚀&nbsp;<a href="#-installation"><code>Installation</code></a>
    </td>
    <td align="center" width="33%">
       🧪&nbsp;<a href="#-examples--tests"><code>Examples&nbsp;&&nbsp;Tests</code></a>
    </td>
    <td align="center" width="33%">
       📚&nbsp;<a href="#-documentation"><code>Documentation</code></a>
    </td>
  </tr>
  <tr>
    <td align="center">
       🤝&nbsp;<a href="#-contributing"><code>Contributing</code></a>
    </td>
    <td align="center">
      🛠️&nbsp;<a href="#-code-standards"><code>Code&nbsp;Standards</code></a>
    </td>
    <td align="center">
      ⚡&nbsp;<a href="#-benchmarks"><code>Benchmarks</code></a>
    </td>
  </tr>
  <tr>
    <td align="center">
      🤖&nbsp;<a href="#-ai-usage--assistant-guidelines"><code>AI&nbsp;Usage</code></a>
    </td>
    <td align="center">
       ⚖️&nbsp;<a href="#-license"><code>License</code></a>
    </td>
    <td align="center">
       👥&nbsp;<a href="#-maintainers"><code>Maintainers</code></a>
    </td>
  </tr>
</table>
<br/>

## 🧩 About

**go-foundation** is the shared, **domain-agnostic** foundation kit used across several Go
services and projects. It exists to kill drift: instead of every service re-implementing the same
configuration, HTTP, persistence, and observability plumbing, that plumbing lives here once
and is consumed everywhere.

The module carries only generic building blocks — no business domain, no project-specific
naming. As the kit is assembled it exposes focused sub-packages:

- **`config`** — application, database, logging, and AWS configuration types
- **`lambda`** — AWS Lambda (API Gateway v2) ⇄ `net/http` adapter
- **`middleware`** — logging, recovery, and request-ID HTTP middleware
- **`ctxutil`** — request-ID context propagation helpers
- **`httputil`** — JSON response and error helpers
- **`pagination`** — cursor-based list pagination
- **`cache`** — generic two-tier TTL cache for validating opaque secrets (bounded, DoS-guarded, injectable clock)
- **`recurrence`** — DST-correct next-occurrence calculator for weekly recurring event patterns
- **`backoff`** — exponential retry-delay ladder (base, doubling, capped)
- **`models`** — generic `BaseModel`, `Repository`, `Clock`, and transaction helpers
- **`secrets`** — pluggable secret providers (env, AWS, mock)
- **`db`** — database connection helpers
- **`health`** — health-check helpers
- **`observability`** — structured logging initialization
- **`testutil`** — dependency-light generic test helpers (in-memory test database, test config, HTTP doer double, log recorder, free port); the testcontainers-backed PostgreSQL harness lives in the `testutil/pgtest` subpackage

> Project-specific naming — environment prefixes, database names, health messages, and infrastructure constants — intentionally stays in the consuming services, never in this module.

<br/>

## 📦 Installation

**go-foundation** requires a [supported release of Go](https://golang.org/doc/devel/release.html#policy).
```shell script
go get -u github.com/mrz1836/go-foundation
```

Get the [MAGE-X](https://github.com/mrz1836/mage-x) build tool for development:
```shell script
go install github.com/mrz1836/mage-x/cmd/magex@latest
```

<br/>

## 📚 Documentation

- **API Reference** – Dive into the godocs at [pkg.go.dev/github.com/mrz1836/go-foundation](https://pkg.go.dev/github.com/mrz1836/go-foundation)
- **Benchmarks** – Browse the [benchmark catalog](#benchmark-catalog) and compare runs with the [benchmark results](#benchmark-results) workflow
- **Test Suite** – Review both the [unit tests](foundation_test.go) (powered by [`testify`](https://github.com/stretchr/testify))

<br/>

<details>
<summary><strong><code>Repository Features</code></strong></summary>
<br/>

This repository includes 25+ built-in features covering CI/CD, security, code quality, developer experience, and community tooling.

**[View the full Repository Features list →](.github/docs/repository-features.md)**

</details>

<details>
<summary><strong><code>Library Deployment</code></strong></summary>
<br/>

This project uses [goreleaser](https://github.com/goreleaser/goreleaser) for streamlined binary and library deployment to GitHub. To get started, install it via:

```bash
brew install goreleaser
```

The release process is defined in the [.goreleaser.yml](.goreleaser.yml) configuration file.


Then create and push a new Git tag using:

```bash
magex version:bump push=true bump=patch branch=master
```

This process ensures consistent, repeatable releases with properly versioned artifacts and metadata.

</details>

<details>
<summary><strong><code>Pre-commit Hooks</code></strong></summary>
<br/>

Set up the Go-Pre-commit System to run the same formatting, linting, and tests defined in [AGENTS.md](.github/AGENTS.md) before every commit:

```bash
go install github.com/mrz1836/go-pre-commit/cmd/go-pre-commit@latest
go-pre-commit install
```

The system is configured via modular env files in [`.github/env/`](.github/env/README.md) and provides 17x faster execution than traditional Python-based pre-commit hooks. See the [complete documentation](http://github.com/mrz1836/go-pre-commit) for details.

</details>

<details>
<summary><strong><code>GitHub Workflows</code></strong></summary>
<br/>

All workflows are driven by modular configuration in [`.github/env/`](.github/env/README.md) — no YAML editing required.

**[View all workflows and the control center →](.github/docs/workflows.md)**

</details>

<details>
<summary><strong><code>Updating Dependencies</code></strong></summary>
<br/>

To update all dependencies (Go modules, linters, and related tools), run:

```bash
magex deps:update
```

This command ensures all dependencies are brought up to date in a single step, including Go modules and any tools managed by [MAGE-X](https://github.com/mrz1836/mage-x). It is the recommended way to keep your development environment and CI in sync with the latest versions.

</details>

<details>
<summary><strong><code>Build Commands</code></strong></summary>
<br/>

View all build commands

```bash script
magex help
```

</details>

<br/>

## 🧪 Examples & Tests

All unit tests run via [GitHub Actions](https://github.com/mrz1836/go-foundation/actions) and use [Go version 1.26.x](https://go.dev/doc/go1.26). View the [configuration file](.github/workflows/fortress.yml).

Run all tests (fast):

```bash script
magex test
```

Run all tests with race detector (slower):
```bash script
magex test:race
```

<br/>

## ⚡ Benchmarks

Every performance-sensitive path ships with a Go benchmark so changes can be **measured, not guessed**. Run them all:

```bash script
magex bench
```

…or with the standard toolchain (all packages, or a single one):

```bash
go test -bench=. -benchmem ./...
go test -bench=. -benchmem ./cache/...
go test -bench=BenchmarkCache_Eviction -benchmem ./cache/...
```

### Benchmark catalog

Every benchmark in the module, linked to its source. The name links jump straight to the function so you can see exactly what is measured.

| Package | Benchmark | Measures |
|---------|-----------|----------|
| `backoff` | [Exponential ladder](backoff/backoff_test.go#L86) | Retry delay at attempts 1 / 5 / 30 |
| `cache` | [Cache hit](cache/cache_test.go#L799) | Revalidating an already-cached secret (fast path) |
| `cache` | [Cache miss](cache/cache_test.go#L816) | Validating an unknown secret (loader + match scan) |
| `cache` | [Parallel reads](cache/cache_test.go#L831) | Concurrent cache hits under the read lock |
| `cache` | [Eviction](cache/cache_test.go#L853) | Evicting the oldest ~10% of a full cache |
| `config` | [Load from env](config/load_test.go#L298) | Reflection-based env binding into a config struct |
| `ctxutil` | [Request ID → metadata](ctxutil/metadata_test.go#L127) | JSON merge + marshal on the request-id stamping path |
| `ctxutil` | [Request ID ← metadata](ctxutil/metadata_test.go#L139) | JSON unmarshal on the request-id extraction path |
| `httputil` | [Write JSON](httputil/httputil_test.go#L196) | Marshal + write of a JSON response body |
| `lambda` | [API Gateway → net/http](lambda/adapter_test.go#L282) | Full request/response adapter round-trip |
| `middleware` | [Logging · large 200](middleware/logging_test.go#L481) | Logging a large successful response body |
| `middleware` | [Logging · request path](middleware/logging_test.go#L512) | Request/response log pair on the happy path |
| `middleware` | [Logging · error response](middleware/logging_test.go#L543) | Capturing + logging a 4xx/5xx body |
| `models` | [Normalize email](models/email_test.go#L198) | Email parse + normalization |
| `models` | [Normalize phone](models/phone_test.go#L78) | Phone parse + normalization |
| `pagination` | [Encode cursor](pagination/pagination_test.go#L102) | Encoding a timestamp cursor |
| `pagination` | [Decode cursor](pagination/pagination_test.go#L112) | Decoding a cursor string |
| `recurrence` | [Parse HH:MM](recurrence/recurrence_internal_test.go#L51) | Hand-rolled start-time parse (hot path) |
| `recurrence` | [Parse pattern](recurrence/recurrence_internal_test.go#L60) | JSON decode of a recurrence pattern |
| `recurrence` | [Next occurrence](recurrence/recurrence_internal_test.go#L72) | Full parse + next-occurrence computation |

### Benchmark results

To measure the impact of a change, capture a baseline before and after and compare with [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) — it reports the delta and whether it is statistically significant, so you are not chasing noise:

```bash
go install golang.org/x/perf/cmd/benchstat@latest

# 1) baseline (e.g. on master), several runs for a stable distribution
go test -bench=. -benchmem -count=6 ./... > old.txt

# 2) after your change, from the same machine
go test -bench=. -benchmem -count=6 ./... > new.txt

# 3) compare
benchstat old.txt new.txt
```

Absolute `ns/op` depends on the host, so treat the numbers below as a **point-in-time reference**, not a contract — the durable signal is the `benchstat` delta between two runs on the same machine.

<details>
<summary><strong>Reference snapshot</strong> — Go 1.27.1 · Apple M1 Max · 2026-09-24</summary>
<br/>

| Benchmark | ns/op | B/op | allocs/op |
|-----------|------:|-----:|----------:|
| Exponential ladder (attempt=1) | 0.63 | 0 | 0 |
| Exponential ladder (attempt=5) | 2.55 | 0 | 0 |
| Exponential ladder (attempt=30) | 3.83 | 0 | 0 |
| Cache hit | 156 | 128 | 2 |
| Cache miss | 190 | 152 | 3 |
| Parallel reads | 153 | 128 | 2 |
| Eviction | 507,752 | 122,880 | 1 |
| Load from env | 3,836 | 1,440 | 37 |
| Request ID → metadata | 909 | 593 | 16 |
| Request ID ← metadata | 279 | 16 | 1 |
| Write JSON | 716 | 1,129 | 15 |
| API Gateway → net/http | 818 | 1,640 | 17 |
| Logging · large 200 | 491,184 | 10,493,671 | 34 |
| Logging · request path | 3,418 | 6,872 | 36 |
| Logging · error response | 3,958 | 7,222 | 43 |
| Normalize email | 641 | 208 | 9 |
| Normalize phone | 577 | 73 | 5 |
| Encode cursor | 17.9 | 16 | 1 |
| Decode cursor | 23.2 | 16 | 1 |
| Parse HH:MM | 24.5 | 0 | 0 |
| Parse pattern | 334 | 48 | 1 |
| Next occurrence | 408 | 48 | 1 |

</details>

<br/>

## 🛠️ Code Standards
Read more about this Go project's [code standards](.github/CODE_STANDARDS.md).

<br/>

## 🤖 AI Usage & Assistant Guidelines
Read the [AI Usage & Assistant Guidelines](.github/tech-conventions/ai-compliance.md) for details on how AI is used in this project and how to interact with the AI assistants.

<br/>

## 👥 Maintainers
| [<img src="https://github.com/mrz1836.png" height="50" width="50" alt="MrZ" />](https://github.com/mrz1836) |
|:-----------------------------------------------------------------------------------------------------------:|
|                                      [MrZ](https://github.com/mrz1836)                                      |

<br/>

## 🤝 Contributing
View the [contributing guidelines](.github/CONTRIBUTING.md) and please follow the [code of conduct](.github/CODE_OF_CONDUCT.md).

### How can I help?
All kinds of contributions are welcome :raised_hands:!
The most basic way to show your support is to star :star2: the project, or to raise issues :speech_balloon:.
You can also support this project by [becoming a sponsor on GitHub](https://github.com/sponsors/mrz1836) :clap:
or by making a [**bitcoin donation**](https://mrz1818.com/?tab=tips&utm_source=github&utm_medium=sponsor-link&utm_campaign=go-foundation&utm_term=go-foundation&utm_content=go-foundation) to ensure this journey continues indefinitely! :rocket:

[![Stars](https://img.shields.io/github/stars/mrz1836/go-foundation?label=Please%20like%20us&style=social&v=1)](https://github.com/mrz1836/go-foundation/stargazers)

<br/>

## 📝 License

[![License](https://img.shields.io/github/license/mrz1836/go-foundation.svg?style=flat&v=1)](LICENSE)
