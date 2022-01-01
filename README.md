# dcache

CoreDNS Plugin: Asynchronous Distributed Cache for Distributed System.

## Name

*dcache* - enables a networked async cache using Redis Pub/Sub.

dcache is a [d]istributed [cache] plugin. 

## Description

Using dcache, you can use Redis Pub/Sub to asynchronously share name resolution resolved by other nodes.
This means that DNS queries do not use unnecessary communication to retrieve the cache, and it operates with very low latency.
It can be used in conjunction with the [CoreDNS standard cache plug-in](https://coredns.io/plugins/cache/).
The TTL of the cache is the smallest value in the response.

If this plugin is enabled and you cannot connect to Redis, it does nothing and does not interfere with CoreDNS operation.

## What is the difference from redisc?
[redisc](https://github.com/miekg/redis) takes the data to Redis after a DNS query,
but dcache uses Pub/Sub to keep the cache data in memory asynchronously.

dcache provides better latency than redisc in environments with very large DNS queries such as crawling.


## Example
```
. {
    forward . 1.1.1.1
    cache
    dcache example.org:6379
}
```
