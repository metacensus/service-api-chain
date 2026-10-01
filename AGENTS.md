# Working on service-api-chain

Decisions with no single declaration to sit beside. Per-declaration detail is a comment beside the code; the [README](README.md) orients an operator.

## What this service is

HTTP, the auth flow, id and `recorded` minting, password hashing and `Kind`→status mapping live in [`metacensus/api`](https://github.com/metacensus/api)'s `go/service`; this repository owns the store. `internal/wire` is the protocol its two deployables share: `internal/gateway` runs in the API, `internal/chaincode` and `internal/ledger` on the peer. `go list -f '{{.ImportPath}}: {{.Doc}}' ./...` maps the rest.

A route is added in the contract first, then given a store method, a `wire` transaction and a `ledger` method here.

## Rules below the seam

- **Deterministic, one invocation per store method**: `internal/ledger`'s package doc; `internal/chaincode` is held to the same.
- **Keys carry the topic** (`prop[topic, prop]`, `vote[topic, prop, user]`) though every topic shares one channel, so a topic's records stay separable ([#3](https://github.com/metacensus/service-api-chain/issues/3)).
- **Signatures are verified in chaincode, hard**, against the key `key_id` resolves to in world state, under `ALLOWED_ORIGINS`. The author must be the caller.

## Testing against Fabric

| Layer | Runs against |
|---|---|
| `ledger` unit + `storetest` | `memkv`, an in-memory world state with Fabric's composite keys, where a transaction does not read its own writes |
| `gateway` unit + `storetest` | the real client encoding into the real `chaincode.Dispatch` over `memkv` |
| `integration/`, `-tags=adapter` | Microfab, with the chaincode served from the test process: conflicts, and `storetest` over the network |
| `integration/`, `-tags=artifact` | Microfab and both built images on one Docker network, over HTTP |

- **Microfab**: one container with the chaincode-as-a-service builder, so no Fabric binaries or docker-in-docker; `fabric-admin-sdk` deploys from Go. Both live in `integration/`'s own module, which is why the adapter tests sit there rather than beside the client.
- **Prove a promise of `store.go` in `storetest`** in `metacensus/api`, not here. Test here only what is Fabric's: the wire, error classification, determinism.

## Not yet

- **Password hashes** sit in public world state ([#1](https://github.com/metacensus/service-api-chain/issues/1)).
- **Any channel member can invoke the chaincode** under any `callerID` ([#2](https://github.com/metacensus/service-api-chain/issues/2)).
- **Sessions** are `auth.MemorySessions`: a restart logs everyone out and the API runs as one replica. They do not belong on the ledger.
