# BTCPayServer LND 

This repository is used to build LND Docker container images that are distributed with BTCPayServer by default.

Docker images are published to https://hub.docker.com/r/btcpayserver/lnd/

Versions:
 - [0.15.0-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.15.0-beta/images/sha256-cd424119be24f0d37183441c9eb7dc69f41aec5300466772eb2a7240be17c55b?context=explore)
 - [0.14.2-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.14.2-beta/images/sha256-b133dc14f36cd046364767fb876aa625f62987ea9503c85da89b533463a0800b?context=explore)
 - [0.14.1-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.14.1-beta/images/sha256-6f4eb67b9fdf8c956fcf964b150488c07cda00e58cffc6dda0009bb170fb98f6?context=explore)
 - [0.13.3-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.13.3-beta/images/sha256-e48e959c47661c8818e8aeee33a6e03137e5a085a6e5effcb1ca554ecf69e0ed?context=explore)
 - [Other versions are tagged](https://github.com/btcpayserver/lnd/tags), but obsoleted and not supported.
    - LND version 0.14.0-beta shipped with check that made it incompatable with c-lightning and eclair (https://github.com/lightningnetwork/lnd/issues/5890)
    - All LND versions prior to 0.13.3 contain specification-level vulnerability (https://lists.linuxfoundation.org/pipermail/lightning-dev/2021-October/003257.html)
    - All LND versions prior to 0.7 contain critical vulnerability (https://lists.linuxfoundation.org/pipermail/lightning-dev/2019-September/002174.html)

Each version is marked with appropriate `basedon-vX.X.X-beta` tags. We are using `basedon` prefix in order not to conflict with LND tags from source repository.

## Source repository

https://github.com/lightningnetwork/lnd

## Links
* [BTCPayServer main repo](https://github.com/btcpayserver/btcpayserver)
* [BTCPayServer-Docker repo](https://github.com/btcpayserver/btcpayserver-docker)
* [BTCPayServer.Lightning](https://github.com/btcpayserver/BTCPayServer.Lightning)
