# service-api-chain

The MetaCensus API backend that persists to Hyperledger Fabric. It implements [`metacensus/api`](https://github.com/metacensus/api)'s `store.Store` across two deployables: a gateway client inside the API process, and chaincode on the peers.
