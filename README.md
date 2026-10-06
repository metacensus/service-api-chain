# service-api-chain

The MetaCensus API over Hyperledger Fabric. It implements [`metacensus/api`](https://github.com/metacensus/api)'s persistence port, `store.Store`, and serves it through that module's shared server, `go/service`. Its sibling, [`service-api-standard`](https://github.com/metacensus/service-api-standard), serves the same contract over Postgres.

The store spans two deployables, built from this one module:

- **the API** (`cmd/service`, `Dockerfile` target `service`), which reaches the peers through a Fabric gateway;
- **the chaincode** (`cmd/chaincode`, target `chaincode`), chaincode-as-a-service the peers dial.

Users and topics live on `FABRIC_CHANNEL`. Each topic's props and votes are meant for a channel of their own (`internal/channels`); until infra's Temporal workflows create channels, the API keeps every topic on that one channel.

Design decisions are in [AGENTS.md](AGENTS.md).

## Endpoints

- **`/metacensus/api/v1/…`** — the contract's authenticated surface; its route manifest is `go/server/routes` in `metacensus/api`.
- **`GET /healthz`** — liveness for the container runtime, outside the prefix; it does not touch the peer.

## Configuration

The API:

| Variable | |
|---|---|
| `FABRIC_PEER_ENDPOINT` | The peer gateway, `host:port`. |
| `FABRIC_PEER_TLS_CA` | PEM CA for the peer's TLS. Required unless `FABRIC_PEER_PLAINTEXT=1`. |
| `FABRIC_PEER_AUTHORITY` | Optional gRPC authority / TLS server name override. |
| `FABRIC_MSP_ID`, `FABRIC_CERT`, `FABRIC_KEY` | The identity every transaction is submitted under: its MSP and PEM cert and key paths. |
| `FABRIC_CHANNEL`, `FABRIC_CHAINCODE` | The channel users and topics live on, and the chaincode's name there and on every topic channel. |
| `PORT` | Default `3001`. |

The chaincode:

| Variable | |
|---|---|
| `CHAINCODE_ID` | The package id the peer installed. |
| `CHAINCODE_SERVER_ADDRESS` | `host:port` to listen on. |
| `ALLOWED_ORIGINS` | Comma-separated WebAuthn origins a participant signature may name. |
| `CHAINCODE_TLS_CERT`, `CHAINCODE_TLS_KEY`, `CHAINCODE_TLS_CLIENT_CA` | Server TLS, optionally mutual. Required unless `CHAINCODE_PLAINTEXT=1`. |
| `GLOBAL_CHANNEL`, `GLOBAL_CHAINCODE` | The channel users and topics live on, and this chaincode's name there, which a topic channel reads keys and topics from. Set together, or neither when every channel is its own. |

## Tests

`make help` lists the targets; `make check` runs what CI's build runs.

## Releasing

`make release-patch` / `-minor` / `-major` tag and push; the tag publishes `docker.io/metacensus/service-api-chain` and `service-api-chain-chaincode`. Publishing needs the `DOCKERHUB_TOKEN` secret, an organization token with write access to both.
