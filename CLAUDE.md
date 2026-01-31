# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

jxt-core is an enterprise-grade microservices foundation framework for Go, providing core components for building modern distributed applications. It is part of the JXT Evidence System - a microservices-based digital evidence management platform.

**Key Technologies**: Go 1.23+, Gin, GORM, Kafka/RedPanda or NATS JetStream, ETCD

**Environment Requirements**:
- Go 1.23+
- Redis 6.0+ (optional)
- MySQL 8.0+ / PostgreSQL 12+ (optional)
- ETCD 3.5+ (for service discovery and multi-tenant config)
- Ginkgo/Gomega (test framework)

---

## Core Components

### EventBus (`sdk/pkg/eventbus/`)

The central event bus supporting Kafka, NATS JetStream, and Memory with unified API.

**Critical Architecture - Hollywood Actor Pool**:
- All three implementations (Kafka/NATS/Memory) use the same Actor Pool with 256 Actors
- Two subscription modes with different routing strategies:

| Mode | Method | Routing | Ordering | Semantics | Use For |
|------|--------|---------|----------|-----------|---------|
| **Domain Events** | `SubscribeEnvelope()` | Consistent hash (by AggregateID) | ✅ Ordered | At-Least-Once | Critical business events (`domain.*` topics) |
| **Regular Messages** | `Subscribe()` | Round-Robin | ❌ Unordered | At-Most-Once | Notifications, cache invalidation |

**Topic Naming Convention**:
- `domain.{aggregate}.{event}` → Use `SubscribeEnvelope()` for ordered, reliable processing
- Other patterns → Use `Subscribe()` for high-throughput, unordered processing

**Code Example**:
```go
// Domain event - guarantees order per aggregate
bus.SubscribeEnvelope(ctx, "domain.order.created", func(ctx context.Context, env *eventbus.Envelope) error {
    // env.AggregateID routes to fixed Actor via consistent hash
    return nil
})

// Regular message - maximum throughput
bus.Subscribe(ctx, "cache.invalidate", func(ctx context.Context, msg []byte) error {
    return nil
})
```

### Outbox Pattern (`sdk/pkg/outbox/`)

Implements the Outbox pattern for reliable event publishing with atomic business operations and event publishing.

**Integration Pattern**: Create an EventBus adapter to bridge jxt-core EventBus with Outbox:
```go
type EventBusAdapter struct { eventBus eventbus.EventBus }

func (a *EventBusAdapter) Publish(ctx context.Context, topic string, data []byte) error {
    return a.eventBus.Publish(ctx, topic, data)
}
```

### Tenant Provider (`sdk/pkg/tenant/`)

ETCD-based multi-tenant configuration management with automatic reconnection and persistent cache fallback.

**Architecture**:
- ETCD (source) → Provider (atomic.Value cache) → Cache Layer (database/ftp/storage)
- CQRS separation: tenant-service manages ETCD write, microservices read via Provider
- Automatic reconnection with exponential backoff (1s → 30s)
- Local file cache fallback when ETCD unavailable

**Middleware** (`sdk/pkg/tenant/middleware/`):
- Supports 4 resolver types: `host` (subdomain), `header`, `query`, `path`
- Helper functions: `GetTenantID()`, `GetTenantIDAsInt()`, `MustGetTenantID()`

---

## Testing

### Test Framework: Ginkgo + Gomega

The project uses Ginkgo v2 for BDD-style testing.

**Running Tests**:
```bash
# From component directory (e.g., sdk/pkg/tenant/middleware)
go test -v

# Run specific test
go test -v -run TestExtractTenantID_Host

# Run all tests from a directory
go test ./...

# Integration tests (require external services like ETCD)
go test -tags=integration
```

**Test Organization**:
- `tests/eventbus/function_tests/` - Functional tests
- `tests/eventbus/performance_regression_tests/` - Performance benchmarks
- `tests/eventbus/reliability_regression_tests/` - Fault tolerance tests
- `sdk/pkg/tenant/integration_reliability_test.go` - Integration tests with build tag

---

## Architecture Patterns

### Domain-Driven Design (DDD)

The codebase follows DDD patterns in command services:
- **Aggregates** - Business entities with consistency boundaries
- **Domain Services** - Business logic that doesn't belong to a single aggregate
- **Value Objects** - Immutable values defined by attributes
- **Domain Events** - Events representing state changes (`sdk/pkg/domain/event/`)

### CQRS (Command Query Responsibility Segregation)

- **Command Side** (Write): Follows DDD with aggregates, domain services
- **Query Side** (Read): Optimized for read performance, does NOT follow DDD
- Events flow from Command to Query via EventBus/Outbox

### Hexagonal Architecture

Services organized around domain logic:
```
internal/
├── domain/         # Core business logic
├── application/    # Application services
├── infrastructure/ # External concerns (DB, EventBus, etc.)
└── interface/      # External interfaces (REST, gRPC)
```

---

## Common Development Commands

Since this is a library/framework, build commands depend on the consuming service. Typical service commands:

```bash
# Build (from service directory)
make b

# Run database migration
make m

# Run service directly
./service-executable server -c config/settings.yml

# Docker build
docker-compose up -d --build
```

---

## Key Documentation References

- [EventBus Architecture](sdk/pkg/eventbus/README.md) - Message routing, Actor Pool, subscription patterns
- [Tenant Module](sdk/pkg/tenant/README.md) - Multi-tenant configuration management
- [Outbox Quick Start](docs/outbox-pattern-quick-start.md) - 5-minute integration guide
- [Main README](README.md) - Project overview and configuration
