# service-api-chain

The MetaCensus API over Hyperledger Fabric. It implements [`metacensus/api`](https://github.com/metacensus/api)'s persistence port, `store.Store`, and serves it through that module's shared server, `go/service`. Its sibling, [`service-api-standard`](https://github.com/metacensus/service-api-standard), serves the same contract over Postgres.

The store spans two deployables, built from this one module:

- **the API** (`cmd/service`, `Dockerfile`), which reaches the peers through a Fabric gateway;
- **the chaincode** (`cmd/chaincode`, `Dockerfile.chaincode`), chaincode-as-a-service the peers dial.

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
| `FABRIC_CHANNEL`, `FABRIC_CHAINCODE` | Where the store lives. |
| `PORT` | Default `3001`. |

The chaincode:

| Variable | |
|---|---|
| `CHAINCODE_ID` | The package id the peer installed. |
| `CHAINCODE_SERVER_ADDRESS` | `host:port` to listen on. |
| `ALLOWED_ORIGINS` | Comma-separated WebAuthn origins a participant signature may name. |
| `CHAINCODE_TLS_CERT`, `CHAINCODE_TLS_KEY`, `CHAINCODE_TLS_CLIENT_CA` | Server TLS, optionally mutual. Required unless `CHAINCODE_PLAINTEXT=1`. |

Both refuse to start on invalid configuration, reporting every problem at once.

## Tests

`make test` runs the unit tests. `make test-integration` and `make test-artifact` run on [Microfab](https://github.com/hyperledger-labs/microfab) and need Docker. `make help` lists every target; `make check` runs what CI's build runs.

## Releasing

`make release-patch` / `release-minor` / `release-major` tag and push; `release.yml` re-runs CI at the tag and publishes `docker.io/metacensus/service-api-chain` and `docker.io/metacensus/service-api-chain-chaincode` for amd64 and arm64. It needs the `DOCKERHUB_TOKEN` secret, a Docker Hub organization access token with write access to both repositories.
