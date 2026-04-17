# SpecGuard Agent

LangChain-powered AI agent for API governance tasks. Wraps the `specguard` CLI as tool calls, enabling natural language interaction with the full SpecGuard pipeline.

## Setup

```bash
pip install -r agent/requirements.txt
export OPENAI_API_KEY="sk-..."
```

Ensure the `specguard` binary is built and on your PATH:

```bash
cd /path/to/specguard
go build -o specguard ./cmd/specguard/
export PATH="$PWD:$PATH"
```

## Usage

### Python API

```python
from agent.agent import create_agent, run_agent

agent = create_agent()
response = run_agent(agent, "Scan /path/to/repo and generate a risk report")
print(response)
```

### CLI

```bash
python -m agent.agent "Scan /path/to/repo and show standards violations"
```

## Available Tools

| Tool | Description |
| --- | --- |
| `specguard_init` | Initialize a SpecGuard project |
| `specguard_scan` | Scan repo for API specs, create snapshots |
| `specguard_diff` | Compare two spec snapshots |
| `specguard_report` | Generate all reports (standards, risk, drift, doc consistency) |
| `read_report_file` | Read a specific report artifact |
| `list_report_artifacts` | List all generated artifacts |

## Architecture

```
User Query → LangChain ReAct Agent → Tool Calls → specguard CLI → Reports
                  ↑                                                    ↓
                  └──────────── Read & Summarize ←─────────────────────┘
```

The agent uses a ReAct (Reasoning + Acting) loop:
1. Receives a natural language query
2. Reasons about which tools to call
3. Executes specguard CLI commands via subprocess
4. Reads generated reports
5. Synthesizes findings into a human-readable response
