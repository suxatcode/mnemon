"""Unified LLM calling via CLIProxyAPI (OpenAI-compatible endpoint).

CLIProxyAPI at http://127.0.0.1:3456 routes requests to the appropriate
upstream provider (Anthropic, OpenAI, etc.) based on model name.
All models use the same OpenAI-compatible /v1/chat/completions endpoint.

IMPORTANT: We bypass system proxy (Surge) for localhost connections
via NO_PROXY to avoid request interception.
"""

import asyncio
import os
import sys

# Bypass Surge proxy for localhost — must be set before any HTTP client init
os.environ.setdefault("NO_PROXY", "127.0.0.1,localhost")
os.environ.setdefault("no_proxy", "127.0.0.1,localhost")

_MAX_RETRIES = 6
_RETRY_DELAY = 10  # seconds (exponential: 10, 20, 40, 80, 160, 320)

_client_cache: dict[str, object] = {}


async def call_llm(
    prompt: str,
    api_base: str,
    api_key: str,
    model: str,
    temperature: float = 0.0,
    max_tokens: int = 1024,
) -> str:
    """Call an LLM via CLIProxyAPI's OpenAI-compatible endpoint.

    Retries up to 3 times on transient errors (500, EOF, timeout).
    """
    try:
        import httpx
        from openai import AsyncOpenAI
    except ImportError:
        raise ImportError("pip install openai httpx  # required for LLM calls")

    # Cache clients by (api_base, api_key) to reuse connections
    cache_key = f"{api_base}:{api_key}"
    if cache_key not in _client_cache:
        http_client = httpx.AsyncClient(
            proxy=None,
            timeout=httpx.Timeout(90.0, connect=10.0),
        )
        _client_cache[cache_key] = AsyncOpenAI(
            api_key=api_key,
            base_url=f"{api_base}/v1" if not api_base.endswith("/v1") else api_base,
            http_client=http_client,
        )

    client = _client_cache[cache_key]

    last_err = None
    for attempt in range(_MAX_RETRIES):
        try:
            response = await client.chat.completions.create(
                model=model,
                temperature=temperature,
                max_tokens=max_tokens,
                messages=[{"role": "user", "content": prompt}],
            )
            return response.choices[0].message.content
        except Exception as e:
            last_err = e
            if attempt < _MAX_RETRIES - 1:
                wait = _RETRY_DELAY * (2 ** attempt)  # exponential: 10, 20, 40, 80, 160, 320
                print(f"    [retry {attempt+1}/{_MAX_RETRIES}] {type(e).__name__}: {e} (wait {wait}s)",
                      file=sys.stderr, flush=True)
                await asyncio.sleep(wait)
    raise last_err
