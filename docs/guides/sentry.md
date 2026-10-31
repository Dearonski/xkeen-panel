# A site that opens only through some servers

The panel shows a working proxy, most sites open, but one keeps answering 403
or an "access denied" page — and on another try it loads fine. The pool is
moving you between servers, and the site accepts only some of them.

Sentry is the usual example. Some providers send traffic bound for Google's
networks through a second exit, such as Cloudflare WARP or an address that
geolocates to another country. Sentry sits behind Google's load balancer, so
it sees that second address, and it bans most of them: its web UI answered
403 through most servers of a subscription while event ingest and the API
worked through all of them.

## Add the site

**Серверы → Сайты → Добавить сайт** (Servers → Sites → Add site)

| Field | Value |
| --- | --- |
| Домены (Domains) | `sentry.io sentry-cdn.com sentry.dev` |
| IP-адреса (IP addresses) | leave empty |
| Адрес проверки (Check address) | `https://sentry.io/auth/login/` |

- Subdomains are included: `sentry.io` also covers `us.sentry.io` and the
  rest.
- Leave the IP addresses empty: Sentry shares Google's addresses with
  unrelated services, which would follow it onto the same servers.
- The check address is the page the ban actually hits. The front page of a
  site often loads through a banned server while the login page does not.

## What the panel does

It opens the check address through every pool server. A server that answers
below 400 passes; the list shows each server's result, so you can see the ban
for yourself. The site then goes only through the servers that passed, and
the pool keeps the rest of your traffic as before.

- The site is rechecked when the pool gains a server and every 12 hours, since
  a server that passes today can be banned tomorrow.
- If no server passes, the site keeps the whole pool rather than being cut
  off.
- A hand-written pool rule for the same domains gives them up to the site.

## Picking a check address for another site

Use a page that fails through the bad servers and opens through the good ones,
such as a login page. Redirects are followed and the final page's status
counts. Anything that answers 4xx through every server — a front page behind
a browser challenge, an API endpoint asking for a key — makes every server
fail and leaves the site on the whole pool.

If the site must also keep one address, lock it to a single server instead —
see [Claude Code through one exit IP](claude-code.md).
