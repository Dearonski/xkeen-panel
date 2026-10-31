# Claude Code through one exit IP

Claude Code holds long sessions and does not like its address changing under
it. Through a pool, every node exits from a different IP, often in a different
country. This guide keeps all Anthropic traffic on one node and cuts it off,
instead of moving it elsewhere, if that node's exit ever changes.

## 1. Add the site

**Серверы → Сайты → Добавить сайт** (Servers → Sites → Add site)

| Field | Value |
| --- | --- |
| Домены (Domains) | `anthropic.com claude.ai claude.com` |
| IP-адреса (IP addresses) | `160.79.104.0/23 2607:6bc0::/48` |
| Адрес проверки (Check address) | `https://console.anthropic.com/` |

- The IP ranges are Anthropic's own. They catch connections whose domain Xray
  could not sniff, and they are what the router-level kill switch blocks while
  the core restarts — a site without them has no protection during a restart.
- The check address must sit on one of the site's domains and behind
  Cloudflare: the panel reads the exit IP from its `/cdn-cgi/trace`. The
  `claude.ai` front page answers 403 to anything that is not a browser and
  would mark every server as failing.

The panel opens the check address through every pool server and shows, next
to each one, the IP and country Anthropic sees.

## 2. Lock it to one server

Open the site, tap a server in **Серверы пула** (Pool servers), then tap
**Закрепить?** (Lock?) to confirm. Prefer the server you have already been
using, so Anthropic sees no change at all; otherwise pick by country. The core
restarts once.

From now on:

- the site goes only through that server, whatever the pool does;
- once a minute the panel checks the exit through the running core;
- if the IP or the country changes, or the server stops answering, the site is
  blocked at once — connections are dropped, never sent through another
  server — and it is let through again only after the exit matches;
- **Принять текущий IP этого сервера** (Accept this server's current IP)
  re-locks a blocked site to its new exit; tapping another server moves the
  lock without a restart.

## 3. Check it

From a computer behind the router:

```sh
curl -s https://api.anthropic.com/cdn-cgi/trace | grep -E '^(ip|loc)='
```

Both lines must match the lock shown in the panel.

## Limits

- **A change can slip through for up to a minute.** If the provider swaps the
  node's exit while the node keeps working, traffic flows from the new address
  until the next check catches it.
- **Telemetry goes elsewhere.** Claude Code also reports to Datadog and Sentry,
  which are not on Anthropic's addresses. Turn that traffic off on the
  computer: `export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`.
- **IPv6 is not proxied by XKeen.** If your devices get global IPv6 addresses,
  their IPv6 traffic bypasses the proxy entirely. The kill switch rejects
  Anthropic's IPv6 range, but other traffic still leaks.
- **A shared node is a shared IP.** Hundreds of other people may exit from the
  same subscription server. A server of your own with a static address is the
  only way to an IP nobody else uses.
