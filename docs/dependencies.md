# M1.1 Dependency Register

Versions are pinned by `server/go.mod`, `server/go.sum`, `server/Dockerfile`, and
`server/docker-compose.yml`. Licence compatibility is recorded for the direct
runtime/build inputs; transitive Go modules remain locked in `go.sum` and are
subject to automated inventory before a public release.

| Dependency | Version | Purpose | Licence | Compatibility |
| --- | --- | --- | --- | --- |
| Go toolchain | 1.24 language baseline; 1.24.7 image | build/runtime language | BSD-3-Clause | permissive |
| `github.com/jackc/pgx/v5` | 5.7.6 | PostgreSQL pool and SQL driver | MIT | permissive |
| `github.com/redis/go-redis/v9` | 9.12.1 | Redis client | BSD-2-Clause | permissive |
| `github.com/pressly/goose/v3` | 3.26.0 | embedded ordered SQL migrations | MIT | permissive |
| PostgreSQL image | 17.6-alpine3.22 | local durable database | PostgreSQL Licence | permissive |
| Redis image | 8.2.1-alpine3.22 | local ephemeral readiness dependency | RSALv2/SSPLv1/AGPLv3 tri-license | local development accepted; production distribution/legal review required |
| Alpine image | 3.22.1 | minimal runtime base | mixed; Alpine package licences | image inventory required before release |

No HTTP framework, ORM, UUID library, configuration decoder, or logging framework
is used. This keeps the runtime dependency surface aligned with ADR-002.
