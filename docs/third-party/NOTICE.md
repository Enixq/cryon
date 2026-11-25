# Cryon Intelligence dependencies

Added production dependencies (versions pinned in go.mod):

| Component | Version | License | Use |
|---|---|---|---|
| [mozilla-ai/any-llm-go](https://github.com/mozilla-ai/any-llm-go/tree/v0.9.0) | v0.9.0 | Apache-2.0 | OpenAI-compatible planner/selector transport |
| [anthropics/anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go/tree/v1.26.0) | v1.26.0 | MIT | Claude transport with explicit HTTP client and no retries |
| [openai/openai-go](https://github.com/openai/openai-go/tree/v1.12.0) | v1.12.0, indirect | Apache-2.0 | SDK used by any-llm-go |

Original license texts are alongside this file. The pinned any-llm-go module
contains no upstream NOTICE file. Cryon does not copy or modify library source.
The Claude adapter uses the official SDK because any-llm-go v0.9.0's Anthropic
adapter does not propagate the supplied HTTP client; the common Director boundary
and strict local plan validation apply to both transports.

No gorse server, vector database, Python, Rust or C++ audio analysis is introduced.
The new AI packages use Go and HTTP. Android retains its existing CGO SQLite
dependency. This file describes additions; the existing application's dependency
licenses remain applicable.
