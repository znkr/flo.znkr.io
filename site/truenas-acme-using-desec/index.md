# TrueNAS ACME using deSEC.io

<!--#meta
    published="2024-07-05"
    summary="""
        TrueNAS only supports two DNS providers out of the box and needs a shell script when you
        want to use a different provider. Unfortunately, there's not a lot of documentation about
        how to write such a shell script.
        """
-->

TrueNAS can secure connections to its WebUI using ACME-provisioned TLS certificates. Because I don't
want to expose TrueNAS to the internet, the best possible way to get a certificate is to use ACME
DNS-01. Unfortunately, TrueNAS only supports Cloudflare and AWS Route 53 out of the box. For other
DNS providers, a shell script can be used. I am using [deSEC.io](http://desec.io) and so I needed to
write a shell script. I struggled quite a bit with this because there's not really any documentation
out there about how to do it.

Assuming TrueNAS tries to get a certificate for `my.dns.domain` it will call the shell script with
the following parameters. To set the challenge, it calls

```shell
myscript.sh set my.dns.domain _acme-challenge.my.dns.domain <challenge>
```

Once the challenge is either solved or failed it removes the challenge from DNS by calling

```shell
myscript.sh unset my.dns.domain _acme-challenge.my.dns.domain <challenge>
```

It doesn't attempt to find the zone for you instead it gives you the full domain name (e.g. let's
say the zone in this example is `dns.domain`, then TrueNAS will provide `my.dns.domain` instead of
`dns.domain`). That's annoying because for deSEC I need the zone. I decided to hard code the zone
into the script to avoid anything more complicated than calling `curl`.

<!--#include-snippet file="truenas_acme.sh" -->

It's not the most robust piece of software, but it does its job.