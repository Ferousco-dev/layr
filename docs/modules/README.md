# M1.1 Module Specifications

These detailed-design specifications implement DES-019..DES-030. Interfaces are conceptual Go signatures until construction; Phase 03 owns exact operator-facing HTTP/CLI contracts.

| File | Detailed elements |
| --- | --- |
| `configuration.md` | DES-019 |
| `observability.md` | DES-020 |
| `postgres.md` | DES-021 |
| `redis.md` | DES-022 |
| `migrations.md` | DES-023 |
| `health.md` | DES-024, DES-025 |
| `middleware.md` | DES-026, DES-027 |
| `httpapi.md` | DES-028 |
| `lifecycle.md` | DES-029 |
| `composition.md` | DES-030 |

No module may import a command package or read process-global environment outside the configuration/composition boundary. Interfaces are declared by consumers at the point of use.

