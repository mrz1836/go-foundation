<div align="center">

# 🚀&nbsp;&nbsp;go-foundation

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
       <a href="https://goreportcard.com/report/github.com/mrz1836/go-foundation"><img src="https://goreportcard.com/badge/github.com/mrz1836/go-foundation?style=flat-square" alt="Go Report"></a>
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
- **`httputil`** — JSON response and error helpers
- **`pagination`** — cursor-based list pagination
- **`models`** — generic `BaseModel`, `Repository`, `Clock`, and transaction helpers
- **`secrets`** — pluggable secret providers (env, AWS, mock)
- **`db`** — database connection helpers
- **`health`** — health-check helpers
- **`observability`** — structured logging initialization
- **`testutil`** — generic test helpers (test database, fixed clock, containers)

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
- **Benchmarks** – Check the latest numbers in the [benchmark results](#benchmark-results)
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

All unit tests run via [GitHub Actions](https://github.com/mrz1836/go-foundation/actions) and use [Go version 1.25.x](https://go.dev/doc/go1.25). View the [configuration file](.github/workflows/fortress.yml).

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

Run the Go benchmarks:

```bash script
magex bench
```

> Benchmarks for the foundation sub-packages are added as those packages are extracted.

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
