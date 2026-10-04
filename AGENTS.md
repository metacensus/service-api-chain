# Working on service-api-chain

Decisions with no single declaration to sit beside. Per-declaration detail is a comment beside the code; the [README](README.md) orients an operator.

## Adding a route

A route starts in the contract and `store.Store`; here it needs a `wire` transaction, a `Dispatch` case, and `gateway` and `ledger` methods — the compiler asks for the methods, `storetest` for the rest. A route on a topic names its channel through `channels.Channels` in `gateway`, and anything its `ledger` method needs from users or topics goes through `ledger.Global`. `go list -f '{{.ImportPath}}: {{.Doc}}' ./...` maps the packages.

## Rules below the seam

- **A topic's props and votes live on the topic's channel**, `metacensus.topic.<uuid>`, named from the id with no lookup; users and topics stay on the global channel ([#3](https://github.com/metacensus/service-api-chain/issues/3), [#7](https://github.com/metacensus/service-api-chain/issues/7)). `internal/channels` is the seam: `Shared` is production until infra's Temporal workflows provision channels, `PerTopic` over `fabrictest.Provision` is the suites.
- **CreateTopic is the one store method that is two steps**: the channel first, then the record on the global channel, so a listed topic takes a prop at once. A refused record leaves its channel behind; infra's orchestration will own the sequence, so nothing here repairs it.
- **A topic channel reads keys and topics across channels** (`ledger.Global`, served by `GetKey` and `GetTopic` on the global channel). Fabric keeps such a read out of the transaction's read set, so it is not re-validated at commit: safe while keys and topics are only ever added.
- **Signatures are verified in chaincode**, under `ALLOWED_ORIGINS` — `go/store`'s promise, kept on the peer, not the gateway.

## Testing against Fabric

| Layer | Runs against |
|---|---|
| `ledger` unit | `memkv` |
| `gateway` unit + `storetest` | the real client encoding into the real `chaincode.Dispatch` over a `memkv` per channel, shared and per topic |
| `gateway`, `-tags=integration` | Microfab, with the chaincode served from the test process: conflicts, the peer's refusal of a channel it has not joined, and `storetest` with a channel created per topic (about 25 a run, 0.4 s each) |
| `integration/`, `-tags=artifact` | Microfab and both built images on one Docker network, over HTTP |

- **[Microfab](https://github.com/hyperledger-labs/microfab)**: one container with the chaincode-as-a-service builder, so no Fabric binaries or docker-in-docker (`internal/fabrictest`). Its orderer runs a system channel, so a channel is created the classic way, a signed update broadcast to the orderer (`fabric-config/configtx`), not the participation API infra targets.
- **Prove a promise of `store.go` in `storetest`** in `metacensus/api`, not here. Test here only what is Fabric's: the wire, error classification, determinism.

## Not yet

- **Password hashes** sit in public world state ([#1](https://github.com/metacensus/service-api-chain/issues/1)).
- **Any channel member can invoke the chaincode** under any `callerID` ([#2](https://github.com/metacensus/service-api-chain/issues/2)).
- **Sessions** are the default `auth.MemorySessions`: a restart logs everyone out and the API runs as one replica ([#5](https://github.com/metacensus/service-api-chain/issues/5)).
