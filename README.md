# BTCPayServer LND 

This repository is used to build LND Docker container images that are distributed with BTCPayServer by default.

Docker images are published to https://hub.docker.com/r/btcpayserver/lnd/

Versions:
 - [0.17.0-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.17.0-beta/images/sha256-2f8d5b9428ecd90606769cdf1bb2063862c0eea115e1328ef7c8b1daf8a42d2d?context=explore)
    - Includes 0.24.1-beta Loop
 - [0.16.4-beta-1](https://hub.docker.com/layers/btcpayserver/lnd/v0.16.4-beta-1/images/sha256-9dd204b62d6c892485b3dd8a76e8f48545ceda5702c9d47329ba4bcbc535a8b4?context=explore)
 - [0.16.3-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.16.3-beta/images/sha256-9ff34769378cfca18664c7d1da3747e7ad7fb7f38a9a7b82a3d4f85e5bfef7bf?context=explore)
 - [0.16.2-beta-1](https://hub.docker.com/layers/btcpayserver/lnd/v0.16.2-beta-1/images/sha256-bfff9de84a0a4af9d643ff555125358861b70374976b970cc00d1e7fc44ed520?context=explore)
 - [0.16.1-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.16.0-beta/images/sha256-f0eb70c20691aaa2ffc34fd5bd6c284299c84e96152cda5e46882a3aa4a3c6a2?context=explore)
 - [0.16.0-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.16.0-beta/images/sha256-f0eb70c20691aaa2ffc34fd5bd6c284299c84e96152cda5e46882a3aa4a3c6a2?context=explore)
 - [0.15.4-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.15.4-beta-1/images/sha256-cadbbff93cf36146e24fa4f32170b4b9d278a2e1acfdc50470790a94506ee9c3?context=explore)
 - [Other versions are tagged](https://github.com/btcpayserver/lnd/tags), but obsoleted and not supported.
    - All LND versions prior to 0.15.4 contain a consensus bug that prevents them from properly parsing transactions with more than 500,000 witness items per input (https://github.com/btcsuite/btcd/issues/1906)
    - All LND versions prior to 0.15.2 contain a bug that prevents them from properly parsing Taproot transactions with script size over 11000 bytes (https://github.com/lightningnetwork/lnd/issues/7002)
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
