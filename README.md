# ⚡ Thor

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-ARM64%20%7C%20x86__64%20%7C%20RISC--V%20%7C%20LoongArch-blue?style=flat-square)](https://github.com/ashir6892/thor/releases)
[![Build](https://img.shields.io/badge/Build-Passing-brightgreen?style=flat-square)](https://github.com/ashir6892/thor/actions)

**Ultra-lightweight personal AI assistant in Go.**  
Runs on $10 hardware with <10MB RAM and boots in under 1 second.

[Quick Start](#-quick-start) · [Providers](#-supported-llm-providers) · [Channels](#-supported-chat-channels) · [Skills](#-skills-system) · [Roadmap](ROADMAP.md)

</div>

---

## ✨ Features

- 🪶 **Ultra-lightweight** — <10MB RAM footprint, <1s boot time, ~8MB binary
- 🔌 **Multi-provider** — OpenAI, Anthropic Claude, GitHub Copilot, Codex CLI, Claude CLI, custom HTTP endpoints
- 💬 **Multi-channel** — Telegram, WhatsApp, Discord, Slack, DingTalk, Feishu/Lark, LINE, QQ, WeChat Work, MaixCAM, Nano
- 🛠️ **Rich built-in tools** — web search, web fetch, shell exec, file ops, cron scheduler, spawn subagents, I2C/SPI hardware control
- 🧩 **Skills system** — extend with SKILL.md files; install from registry with one command
- 🧠 **Persistent memory** — remembers context across sessions via MEMORY.md
- 🤖 **Multi-agent** — spawn background subagents for parallel task execution
- 📦 **Single binary** — zero runtime dependencies, CGO disabled by default
- 🏗️ **Cross-platform** — Linux (amd64, arm64, armv7, riscv64, loong64), macOS, Windows
- 🔒 **PM2 compatible** — production-grade process management with auto-restart

---

## 📊 Comparison

| Feature | legacy | NanoBot | **Thor** |
|---|---|---|---|
| **RAM Usage** | ~200MB | ~80MB | **<10MB** |
| **Boot Time** | ~5s | ~2s | **<1s** |
| **Binary Size** | ~50MB | ~30MB | **~8MB** |
| **Language** | Python | Node.js | **Go** |
| **Dependencies** | Many | Several | **Zero** |
| **Embedded HW** | ❌ | ❌ | **✅ I2C/SPI** |
| **Min Hardware** | RPi 4 | RPi 3 | **$10 SBC** |
| **CGO Required** | Yes | Yes | **No** |

> Thor is designed to run on hardware like the **LicheeRV Nano**, **Raspberry Pi Zero**, Android (Termux), or any Linux system — including those with as little as 64MB RAM.

---

## 🚀 Quick Start

### Installation

**Option 1: Download pre-built binary**

```bash
# Linux arm64 (e.g. Termux, Raspberry Pi, LicheeRV Nano)
curl -L https://github.com/ashir6892/thor/releases/latest/download/thor-linux-arm64 -o thor
chmod +x thor
./thor --version
```

**Option 2: Build from source**

```bash
git clone https://github.com/ashir6892/thor.git
cd thor
go build ./cmd/thor         # Build for current platform
# OR
make build                  # Build with version info
make build-all              # Build for all platforms
```

**Option 3: Install to PATH**

```bash
make install                # Installs to ~/.local/bin/thor
```

### Configuration

```bash
# Copy example config
cp config/config.example.json ~/.thor/config.json

# Edit with your API keys and settings
$EDITOR ~/.thor/config.json
```

**Minimal `config.json`:**

```json
{
  "providers": [
    {
      "type": "anthropic",
      "api_key": "sk-ant-...",
      "model": "claude-opus-4-5"
    }
  ],
  "channels": [
    {
      "type": "telegram",
      "token": "your-bot-token"
    }
  ]
}
```

### Run

```bash
# Start the agent (connects all configured channels)
./thor gateway

# Or with PM2 for production
pm2 start thor -- gateway
pm2 save
```

---

## 🤖 Supported LLM Providers

| Provider | Type | Notes |
|---|---|---|
| **OpenAI** | Cloud | GPT-4o, GPT-4.1, o1, o3, etc. |
| **Anthropic (Claude)** | Cloud | Claude Opus, Sonnet, Haiku |
| **GitHub Copilot** | Cloud | Via Copilot SDK; uses your existing subscription |
| **Codex CLI** | Local CLI | Runs OpenAI Codex via local CLI tool |
| **Claude CLI** | Local CLI | Runs Claude via Anthropic's CLI tool |
| **HTTP/Custom** | Any | Point to any OpenAI-compatible endpoint (Ollama, vLLM, LM Studio, etc.) |
| **Fallback** | Meta | Chain multiple providers; auto-failover on error |
| **Factory** | Meta | Dynamic provider selection based on routing rules |

**Example: Use Ollama locally**

```json
{
  "type": "http",
  "base_url": "http://localhost:11434/v1",
  "model": "llama3.2:3b",
  "api_key": "ollama"
}
```

---

## 💬 Supported Chat Channels

| Channel | Notes |
|---|---|
| **Telegram** | Full bot API support, inline keyboards, media |
| **WhatsApp** | Via WhatsApp Web protocol (whatsmeow) |
| **Discord** | Bot with slash commands and DM support |
| **Slack** | App with socket mode |
| **DingTalk** | Stream SDK, enterprise messaging |
| **Feishu / Lark** | Bytedance enterprise platform |
| **LINE** | Webhook-based messaging |
| **QQ** | Via OneBot protocol (QQ official bot API) |
| **WeChat Work (WeCom)** | Enterprise WeChat integration |
| **MaixCAM** | MaixCAM hardware display channel |
| **Nano** | Minimal embedded display channel |

---

## 🛠️ Built-in Tools

Thor comes with a comprehensive set of tools available to the AI agent:

| Tool | Description |
|---|---|
| `web_search` | Search the web for current information |
| `web_fetch` | Fetch and extract readable content from any URL |
| `exec` | Execute shell commands (with timeout & safety controls) |
| `read_file` | Read file contents |
| `write_file` | Write content to a file |
| `edit_file` | Edit files by replacing specific text |
| `append_file` | Append content to a file |
| `cron` | Schedule one-time or recurring tasks |
| `message` | Send messages to chat channels |
| `spawn` | Spawn background subagents for parallel tasks |
| `i2c` | I2C bus operations (detect, scan, read, write) |
| `spi` | SPI bus operations (list, transfer, read) |
| `find_skills` | Search skill registry for installable skills |
| `install_skill` | Download and install a skill from registry |

### Hardware Tools (I2C / SPI)

Thor can directly control hardware peripherals — perfect for embedded systems:

```
# Detect I2C buses
i2c detect

# Scan for devices on bus 1
i2c scan --bus 1

# Read temperature from sensor at 0x48
i2c read --bus 1 --address 0x48 --register 0x00 --length 2
```

---

## 🧩 Skills System

Skills extend Thor's capabilities with domain-specific knowledge and workflows. Each skill is a directory containing a `SKILL.md` file with instructions for the agent.

### Install from Registry

```bash
# Search for skills
thor skill search weather

# Install a skill
thor skill install weather

# Or via the agent itself
find_skills("github integration")
install_skill("github", registry="clawhub")
```

### Built-in Skills

| Skill | Description |
|---|---|
| `weather` | Real-time weather lookup and forecasts |
| `github` | GitHub repo management, PRs, issues |
| `tmux` | Terminal multiplexer session management |
| `summarize` | Summarize web pages and documents |
| `skill-creator` | Create new skills from natural language |
| `hardware` | Hardware sensor reading and control |

### Create Your Own Skill

```bash
mkdir ~/.thor/workspace/skills/my-skill
cat > ~/.thor/workspace/skills/my-skill/SKILL.md << 'EOF'
# My Custom Skill

## Description
This skill helps with...

## Instructions
When the user asks about X, do Y using the following steps...
EOF
```

Skills are automatically discovered and loaded by Thor on startup.

---

## 🧠 Memory System

Thor maintains persistent memory across sessions using Markdown files in the workspace:

```
~/.thor/workspace/
├── memory/
│   ├── MEMORY.md          # Long-term facts, preferences, config
│   └── YYYYMM/
│       └── YYYYMMDD.md    # Daily chat history
├── skills/                # Installed skills
├── AGENT.md               # Agent behavior instructions
├── IDENTITY.md            # Agent identity/persona
└── SOUL.md                # Agent values and principles
```

**MEMORY.md** stores:
- User information and preferences
- Important facts learned over time
- Confirmed working methods and configurations
- Safety rules and constraints

The agent reads MEMORY.md on startup and updates it during conversations. This gives Thor a persistent "brain" that survives restarts.

---

## 🏗️ Architecture

```
thor/
├── cmd/
│   ├── thor/              # Main binary (agent + gateway)
│   ├── thor-launcher/     # Web UI launcher
│   └── thor-launcher-tui/ # TUI launcher
├── pkg/
│   ├── agent/             # Core agent loop & reasoning
│   ├── providers/         # LLM provider adapters
│   ├── channels/          # Chat channel adapters
│   ├── tools/             # Built-in tool implementations
│   ├── skills/            # Skills loading & management
│   ├── session/           # Session & conversation management
│   ├── routing/           # Message routing logic
│   ├── cron/              # Scheduler implementation
│   ├── bus/               # Internal event bus
│   ├── config/            # Configuration parsing
│   ├── identity/          # Agent identity management
│   ├── memory/            # Memory/state persistence
│   ├── auth/              # Authentication helpers
│   ├── devices/           # Hardware device abstractions
│   ├── voice/             # Voice/audio processing
│   └── utils/             # Shared utilities
└── workspace/             # Default workspace template
    ├── skills/            # Built-in skills
    └── memory/            # Memory templates
```

**Key design principles:**
- **Zero CGO** — pure Go, cross-compiles to any target
- **Plugin architecture** — providers, channels, and tools are all registered via interfaces
- **Event-driven** — internal bus decouples components
- **Workspace-first** — all user data lives in `~/.thor/workspace/`

---

## 📦 Building

```bash
# Current platform
make build

# Specific platforms
make build-linux-arm64      # ARM64 (RPi, LicheeRV Nano, Termux)
make build-linux-arm        # ARMv7 (RPi Zero 32-bit)
make build-all              # All platforms

# With WhatsApp native (larger binary, uses whatsmeow)
make build-whatsapp-native

# Direct go build
CGO_ENABLED=0 go build -o thor ./cmd/thor

# Run tests
make test
go test ./...
```

**Build outputs** go to `build/`:
- `thor-linux-amd64`
- `thor-linux-arm64`
- `thor-linux-arm`
- `thor-linux-riscv64`
- `thor-linux-loong64`
- `thor-darwin-arm64`
- `thor-windows-amd64.exe`

---

## ⚙️ Environment Variables

| Variable | Default | Description |
|---|---|---|
| `THOR_HOME` | `~/.thor` | Thor home directory |
| `THOR_CONFIG` | `$THOR_HOME/config.json` | Config file path |
| `THOR_WORKSPACE` | `$THOR_HOME/workspace` | Workspace directory |
| `THOR_LOG_LEVEL` | `info` | Log level (debug/info/warn/error) |

---

## 🤝 Contributing

We welcome contributions! Thor is ~95% AI-bootstrapped — we use AI agents to write, test, and review code. You can too.

1. **Fork** the repository
2. **Create** a feature branch: `git checkout -b feat/my-feature`
3. **Write** your code and tests
4. **Run** `make test` and `make lint`
5. **Submit** a pull request

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

### Areas where we need help

- 🌐 **New channel adapters** — Signal, Email, KOOK, Matrix
- 🤖 **New LLM providers** — Gemini, Mistral, Groq, local Ollama
- 📖 **Documentation** — platform guides, tutorials, translations
- 🔒 **Security hardening** — sandbox, input validation, SSRF protection
- 🧪 **Tests** — integration tests, fuzzing

See [ROADMAP.md](ROADMAP.md) for the full vision.

---

## 🌍 Translations

| Language | README |
|---|---|
| 🇨🇳 Chinese | [README.zh.md](README.zh.md) |
| 🇯🇵 Japanese | [README.ja.md](README.ja.md) |
| 🇫🇷 French | [README.fr.md](README.fr.md) |
| 🇧🇷 Portuguese (BR) | [README.pt-br.md](README.pt-br.md) |
| 🇻🇳 Vietnamese | [README.vi.md](README.vi.md) |

---

## 📄 License

MIT © [embedded](https://github.com/ashir6892)

See [LICENSE](LICENSE) for details.

---

<div align="center">

**⚡ Built with Go · Runs anywhere · Weighs almost nothing**

*Thor is ~95% AI-bootstrapped — conceived, coded, and iterated by AI agents running on the very hardware it targets.*

[⭐ Star us on GitHub](https://github.com/ashir6892/thor) · [🐛 Report Issues](https://github.com/ashir6892/thor/issues) · [💬 Discussions](https://github.com/ashir6892/thor/discussions)

</div>
