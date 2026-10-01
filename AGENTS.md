# Working on service-api-chain

Decisions with no single declaration to sit beside. Per-declaration detail is a comment beside the code; the [README](README.md) orients an operator.

## What this service is

Everything above `store.Store` is [`metacensus/api`](https://github.com/metacensus/api)'s `go/service`; this repository is the store. `go list -f '{{.ImportPath}}: {{.Doc}}' ./...` maps it.

A route starts in the contract and `store.Store`; here it needs a `wire` transaction, a `Dispatch` case, and `gateway` and `ledger` methods — the compiler asks for the methods, `storetest` for the rest.

## Rules below the seam

- **Keys carry the topic** though every topic shares one channel, so a topic can move to its own ([#3](https://github.com/metacensus/service-api-chain/issues/3)).
- **Signatures are verified in chaincode**, under `ALLOWED_ORIGINS` — `go/store`'s promise, kept on the peer, not the gateway.

## Testing against Fabric

| Layer | Runs against |
|---|---|
| `ledger` unit | `memkv` |
| `gateway` unit + `storetest` | the real client encoding into the real `chaincode.Dispatch` over `memkv` |
| `gateway`, `-tags=integration` | Microfab, with the chaincode served from the test process: conflicts, and `storetest` over the network |
| `integration/`, `-tags=artifact` | Microfab and both built images on one Docker network, over HTTP |

- **[Microfab](https://github.com/hyperledger-labs/microfab)**: one container with the chaincode-as-a-service builder, so no Fabric binaries or docker-in-docker (`internal/fabrictest`).
- **Prove a promise of `store.go` in `storetest`** in `metacensus/api`, not here. Test here only what is Fabric's: the wire, error classification, determinism.

## Not yet

- **Password hashes** sit in public world state ([#1](https://github.com/metacensus/service-api-chain/issues/1)).
- **Any channel member can invoke the chaincode** under any `callerID` ([#2](https://github.com/metacensus/service-api-chain/issues/2)).
- **Sessions** are the default `auth.MemorySessions`: a restart logs everyone out and the API runs as one replica ([#5](https://github.com/metacensus/service-api-chain/issues/5)).
