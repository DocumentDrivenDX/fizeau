"""Fake provider values used by adapter tests without paid API calls."""

PROFILE = {
    "id": "fake-openrouter",
    "provider": {
        "type": "openai-compat",
        "model": "qwen/qwen3.6-plus",
        "base_url": "http://127.0.0.1:65530/v1",
        "api_key_env": "FAKE_PROVIDER_API_KEY",
    },
    "limits": {
        "max_output_tokens": 4096,
        "context_tokens": 131072,
    },
    "sampling": {
        "temperature": 0.0,
        "reasoning": "medium",
    },
}

API_KEY = "fake-secret-key"
