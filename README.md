# AWheel

A spinning wheel. Build a wheel, share one link, and
everybody on the internet watches the same spin at the same moment.

Live at [wheel.akarpov.ru](https://wheel.akarpov.ru).

## Features

- No accounts. A cookie is your identity.
- Wheels with named entries and per-entry weights.
- Two modes:
  - **Selection** — each spin names a winner, nothing is removed.
  - **Exclusion** — the picked entry drops out, spin until one is left.
- Spin length is adjustable, 15 seconds by default.
- One share link. Spectators watch live over server-sent events; only the
  creator can spin.
- The server decides the result and broadcasts the exact rotation, so every
  browser shows the same wheel in the same position, even joining mid-spin.
- Distinct colour per entry, in the wheel and in the list beside it.
- Plain adaptive layout that follows your light or dark system theme.

## Use

1. Open the site and create a wheel.
2. Add entries, one at a time or pasted in bulk (`Bob | 3` sets a weight).
3. Pick a mode and a spin time, tick the entries to include.
4. Press **Start play session** and share the link it gives you.
5. Press **Spin**. Everyone watching sees it turn and stop together.

## Run it

Needs Go 1.22+ and PostgreSQL.

```sh
createdb wheel
cp .env.example .env      # then set DATABASE_URL
make run
```

Or with Docker:

```sh
docker compose up --build
```

The service listens on `ADDR` (`:8080` by default) and applies its own
migrations on start. Every setting lives in `.env`; see `.env.example` for the
full list. Real environment variables always override the file.

## Deploy

`deploy/wheel.service` is a systemd unit and `deploy/nginx.conf` is a reverse
proxy config with the buffering turned off that event streams need. Set
`COOKIE_SECURE=true` and `BASE_URL=https://your.host` behind TLS.
