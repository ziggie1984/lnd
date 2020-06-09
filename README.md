# BTCPayServer LND 

This repository is used to build LND Docker container images that are distributed with BTCPayServer by default.

Docker images are published to https://hub.docker.com/r/btcpayserver/lnd/

Versions:
 - [0.10.1-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.10.1-beta/images/sha256-39903f3ed2317cd62d4afcbcd1f3f063a3baff39b3b5ef8d0537f4006300d77c?context=explore)
 - [0.9.2-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.9.2-beta/images/sha256-02fb48e8f1a3f92cb9ec4b168a0820073a52a9a8ed67279f0d8ea0e465fe15bc?context=explore)
 - [0.8.2-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.8.2-beta/images/sha256-31846e2a8bd347a5da979dda8b7f52babf425e11739fc267bc767194cf02a206?context=explore)
 - [0.8.1-beta](https://hub.docker.com/layers/btcpayserver/lnd/v0.8.1-beta/images/sha256-dcfca21542ef42bb5a52e376d55154ddc8e6b054d006f00ed6982ab801f05a4b?context=explore)
 - Previous versions are tagged, but obsoleted and not supported. Please note that all LND versions prior to 0.7 contain critical vulnerability (https://lists.linuxfoundation.org/pipermail/lightning-dev/2019-September/002174.html)

Each version is marked with appropriate `basedon-vX.X.X-beta` tags. We are using `basedon` prefix in order not to conflict with LND tags from source repository.

## Source repository

https://github.com/lightningnetwork/lnd

## Links
* [BTCPayServer main repo](https://github.com/btcpayserver/btcpayserver)
* [BTCPayServer-Docker repo](https://github.com/btcpayserver/btcpayserver-docker)
