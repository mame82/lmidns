# "Let Me In" rebind DNS server by MaMe82

## Usage:   
`lmidns -domain my-rebind-domain.com`

## Query example (nslookup as resolver):
### 1) Creating a DNS request, which resolves to an arbitrary IPv4 address:                    *
`nslookup 192-168-2-1.my-rebind-domain.com`

The host part of the request will be parsed by the DNS server and handed back as IP in the response

`Response:  192.168.2.1`

### 2) Temporary pin another IP

A request for `192-168-2-1.my-rebind-domain.com` will be resolved to `192.168.2.1`. In order to allow DNS rebinding, this request has to temporarily be resolved to another arbitrary IP (the one, where custom "evil content" is hosted).  
In order to pin another IP, let's say `3.2.4.5` for a given amount of seconds, the following name scheme is used:
`<legacy-ip>-to-<temporary-ip>-for-<time-to-pin-in-seconds>`.

#### Example:

Pin the IP 3.2.4.5 to lookups aiming for IP 192.168.2.1 for 10 seconds

* Request 1: `nslookup 192-168-2-1-to-3-2-4-5-for-10.my-rebind-domain.com`
* Response 1: `3.2.4.5` (this response doesn't help, as the domain conflicts with SOP)

Issue a lookup which should return 192.168.2.1 (if no other IP is pinned)

* Request 2: `nslookup 192-168-2-1.my-rebind-domain.com`
* Response 2: `3.2.4.5` (lookups resolve to 3.2.4.5 for 10 seconds) forwarder themselves.



... wait 10 seconds and repeat the lookup ...

* Request 3:  `nslookup 192-168-2-1.my-rebind-domain.com`
* Response 3: `192.168.2.1` (the lookup resolves to the intended IP again)

So we could resolve the same hostname to two different IP addresses, which allows to bypass SOP in DNS rebinding based attacks.

Usually the first IP delivers a JS payload, which tries to constantly access the same hostname via JS (respecting the SOP). As soon as the hostname resolves to the former arbitrary IP (after timeout), the JS ends up accessing a potential target (still with respect to the SOP). 

## Install
`go get github.com/mame82/lmidns`

`lmidns -domain your.rebind.domain`

## DNS setup example

In order to use this as public DNS, you need to setup a `NS` record and, for example, an `A` record which the `NS` record could point to.

**Example for zone 'evil.com' with `lmidns` running on 2.4.3.5:**

Below is an example for the needed resource records on a zone (owned by you) with the name `evil.com`.
The "Let Me In" rebind DNS server should use the subdomain `lmins.evil.com`

```
lmins.evil.com.		0       IN	A	2.4.3.5
lmi.evil.com.		3600	IN	NS	lmins.evil.com.         
```

With this zone configuration the NS for lmins.evil.com is on 2.4.3.5 and we can run lmidns on this server with:

`lmidns -domain lmins.evil.com`  

## Additional notes

As most of the DNS rebind attacks are targeting with SoHo networks, keep in mind that some routers block DNS replies belonging to their own subnets, in case they're providing a DNS semselves.

F.e. AVM FRITZ!Box provides a subnet like `192.168.7.0/24` for LAN clients and the gateway provides an DNS forwarder on `192.168.178.1:53`. Now if one of the clients of this subnet tries to resolve `192-168-178-1.lmi.evil.com`, the response (which would be `192.168.172.1`) will blocked by the FRITZ!Box DNS forwarder (which is a nice DNS rebinding mitigation btw.). This could only be bypassed, in case the client uses a hardcoded external nameserver (ignoring the one offered by DHCP server).

So please, avoid opening issues for things like that.


