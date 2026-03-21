#!/usr/bin/env bash
# LoCoMo Benchmark runner for mnemon
#
# All LLM calls route through CLIProxyAPI (http://127.0.0.1:3456).
# No API keys needed — CLIProxyAPI handles upstream auth.
#
# Usage:
#   ./bench/run.sh                    # Mode D (raw), all samples
#   ./bench/run.sh --mode full        # Mode E (full LLM-supervised)
#   ./bench/run.sh --mode both        # Run both modes and compare
#   ./bench/run.sh --limit 1 --max-questions 5   # Quick test: 1 sample, 5 questions
#   ./bench/run.sh --clean            # Clean previous data before running
#
# Override models/proxy via env:
#   INGESTION_MODEL=claude-haiku-4-5-20251001  (default)
#   ANSWER_MODEL=claude-sonnet-4-6             (default)
#   INGESTION_API_BASE=http://127.0.0.1:3456   (default)
#   ANSWER_API_BASE=http://127.0.0.1:3456      (default)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Bypass Surge proxy for localhost
export NO_PROXY="127.0.0.1,localhost"
export no_proxy="127.0.0.1,localhost"

# Check CLIProxyAPI
if ! curl --noproxy '*' -sf -H "x-api-key: not-needed" http://127.0.0.1:3456/v1/models >/dev/null 2>&1; then
    echo "Error: CLIProxyAPI not reachable at http://127.0.0.1:3456"
    echo "Start it with: brew services start cliproxyapi"
    exit 1
fi
echo "CLIProxyAPI: OK"

# Check dataset
if [ ! -f testdata/locomo/locomo10.json ]; then
    echo "Downloading LoCoMo dataset..."
    mkdir -p testdata/locomo
    curl -sL -o testdata/locomo/locomo10.json \
        "https://raw.githubusercontent.com/playeriv65/EasyLocomo/master/data/locomo10.json"
    echo "Downloaded ($(wc -c < testdata/locomo/locomo10.json) bytes)"
fi

# Check mnemon
if ! command -v mnemon &>/dev/null; then
    echo "Error: mnemon not found in PATH"
    echo "Run: make install  (from the mnemon project root)"
    exit 1
fi

# Check Python deps
python3 -c "import openai, httpx" 2>/dev/null || {
    echo "Error: missing Python deps. Run: pip install openai httpx"
    exit 1
}

# Run benchmark
python3 -m locomo.run_bench "$@"
