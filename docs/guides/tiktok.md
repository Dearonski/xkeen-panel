# TikTok that keeps showing the Russian feed

The proxy works and TikTok opens, but now and then it serves the frozen Russian
feed: old videos, no new uploads. Through one server it works, through another
it does not, and switching servers does not reliably help.

TikTok decides the region from the address its own servers see, and that is not
always the node's exit. Some providers send traffic for big networks through a
second exit, and TikTok sits on two of them: the web and most of the app API on
Akamai, part of the app API on Google Cloud. A node can look clean to every
echo service and still reach TikTok from a Russian address, or from one that
TikTok's own geo database places in Russia.

So instead of guessing from the node, ask TikTok. Its pages carry the region it
sees you in.

## Add the site

**Серверы → Сайты → Добавить сайт** (Servers → Sites → Add site)

| Field | Value |
| --- | --- |
| Домены (Domains) | `tiktok.com tiktokv.com tiktokv.us tiktokcdn.com tiktokcdn-us.com ttwstatic.com byteoversea.com ibytedtos.com ibyteimg.com` |
| IP-адреса (IP addresses) | leave empty |
| Адрес проверки (Check address) | `https://www.tiktok.com/explore` |
| Страна со слов сайта (Country the site reports) | `"region":"([A-Z]{2})"` |

- The domains cover the web, the app API and the video CDN.
- Leave the IP addresses empty: the CDNs are shared with unrelated sites.
- The pattern picks the two-letter region out of the page; the lowercase
  `"region":"eu-ttp2"` next to it does not match.

## What the panel does

It opens the check address through every server and reads the region TikTok
reports. Each server in the site's list shows it as **сайт видит …** (the site
sees …).

- A server through which TikTok sees a country from **Не выбирать страны**
  (Do not pick countries, in Settings) fails the check, and the site does not
  go through it.
- A server whose page carries no region fails too, so a captcha or an error
  page never passes as clean.
- If no server passes, TikTok keeps the whole pool rather than being cut off.
- With the source **Страны** (Countries), servers are picked by their scanned
  exit, and one through which TikTok sees another country still fails.

Keep **Не выбирать страны** set to `RU`; the same list also keeps auto-pick off
nodes whose Google Cloud exit is Russian, which covers the app API on Google
Cloud.

## Limits

- **The check sees the web, not every app endpoint.** It goes to Akamai, like
  most of the app; the Google Cloud part is covered only by the pool's exit
  scan.
- **The app remembers its region.** If TikTok was opened while traffic went
  through a bad server, restart the app after the site check has run.
- **Exits can change between checks.** The site is rechecked when its servers
  change and every 12 hours; press **Проверить снова** (Check again) if the
  feed freezes in between.
