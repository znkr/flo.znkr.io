~~~
    title: Point-to-Point Routing
published: 2024-07-06
 abstract: Adventures in IP routing when connecting my laptop's dock to my NAS via a 10G
           point-to-point link in parallel to my 1G LAN link. Turns out, it's not too hard
           to use the 10G link transparently.
~~~

## Background

When I upgraded my NAS to have a 10G network interface I didn't have a 10G switch in my network. To
still have a fast 10G connection I installed a direct link between my desk and my TrueNAS Scale box.
That link existed in parallel to my regular 1G LAN link:

![Network topology diagram](/point-to-point-routing/topology.png)

Let's say we use the subnet `10.0.0.0/24` for the LAN and `172.16.0.0/30` for the point-to-point
link [^1]. An easy way to use the 10G link would be to always use my NAS's IP address on the
point-to-point subnet `172.16.0.1` when accessing my NAS. That's a little inconvenient though, I
would prefer to use the hostname which resolves to `10.0.0.2`.

[^1]: I know I could be using a `/31` here, but they are a bit special and I didn't want to run into
any issues. It's unlikely that I'll be using more than 1 point-to-point link ever, so the `/30` is
already more for documentation than to save IP address resources.

If my computer was stationary and always connected to the two links, I could simply override the
hostname (e.g. by editing `/etc/hosts`). However, my computer is MacBook and I use it from the couch
as well as from my desk. When I am at my desk, I use a Thunderbolt dock that's connected to all my
periphery and the two network links. In that case, I want NAS connections to use the 10G link. But
when I am using it from anywhere else, I want to use my Wifi link or VPN.

This is not an impossible setup, IP routing is very powerful and my intuition was that this should
be possible to use the 10G link transparently using IP routing. However, I did not find a guide to
describe how to set this up, and needed to piece it together.

## Point-to-Point Routing

All IP routing works by sending packets intended for one IP address to a different IP address
without actually changing the destination IP address in the IP packet. On ethernet links that
usually means sending these packets to the MAC address.

The receiver of such a packet then decides what to do with that packet. A router will usually
forward the packet to a different router until it reaches its destination.

The most common case is to just send everything that's not on any local subnet to a *default
gateway*. In home networks that's often the router that connects the LAN to the internet and
forwards the packets to the ISPs router.

For point-to-point routing, we add a static entry into the routing table on both ends of the
point-to-point link that routes all packets intended for the other end to the point-to-point subnet
address instead. That is, my MacBook, when connected to the 10G link, should send all IP packets
intended for `10.0.0.2` to `172.16.0.1` and my NAS should send all packets intended for `10.0.0.122`
to `172.16.0.2`.

This is enough because both Linux and MacOS implement the [weak host
model](https://en.wikipedia.org/wiki/Host_model): They don't care at what interface packets arrive.
If a packet for `10.0.0.2` arrives at the interface configured for `172.16.0.1`, both Linux and
MacOS will accept it if a different interface is configured for `10.0.0.2`.

### TrueNAS Scale

On TrueNAS adding a static route is straightforward, specifying the destination as `10.0.0.122/32`
and the gateway as `172.16.0.2` is all that's necessary:

![TrueNAS "Add Static Route" screen](/point-to-point-routing/truenas.png)

The destination `10.0.0.122/32` is CIDR for "only `10.0.0.122`", so only packets intended for that
IP address will be routed to `172.16.0.2`.

### MacOS

The setup on MacOS requires some command-line shenanigans though. First, we need to figure out the
name of the 10G device. The simplest way I found was to run `networksetup` and compare with system
settings:

```plaintext {linenos=false}
% networksetup -listallnetworkservices
An asterisk (*) denotes that a network service is disabled.
USB 10/100/1000 LAN
Thunderbolt Ethernet Slot 0
Wi-Fi
MYVPN
```

In my case "Thunderbolt Ethernet Slot 0" is the 10G point-to-point link and "USB 10/100/1000 LAN" is
the 1G LAN link.

Setting a static route on the "Thunderbolt Ethernet Slot 0"  device works like this:

```plaintext {linenos=false}
% networksetup -setadditionalroutes \
    "Thunderbolt Ethernet Slot 0" \
    10.0.0.2 255.255.255.255 \ 
    172.16.0.1
```

Here, instead of the CIDR notation used by TrueNAS, the subnet mask `255.255.255.255` makes sure to
select only the IP address `10.0.0.2` to route to `172.16.0.1`.

### Caveats

Neither of the two systems allows setting the source IP address. That's a bummer because without
that connections using the 10G link use the point-to-point IP address instead of the LAN address.
However, I haven't run into any issues because of that yet.