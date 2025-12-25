# AoE4 Quote Database

A quote database application for Age of Empires IV streamers and their communities. Collect and share memorable quotes, jokes, and wisdom from the battlefield.

## Features

- **Public API**: Get random quotes as plain text, perfect for chat bots and stream overlays
- **Civ filtering**: Filter quotes by civilization using shortnames (e.g., `?civ=hre`)
- **Matchup tips**: Get tips for specific civ vs civ matchups (e.g., `?civ=hre&vs=french`)
- **Web interface**: Authenticated users can add, view, and delete quotes
- **Civilization management**: Full list of all 22 AoE4 civilizations across all DLCs
- **exe.dev authentication**: Login via exe.dev identity system

## API Endpoints

### Public (no auth required)

| Endpoint | Description |
|----------|-------------|
| `GET /browse` | Browse all quotes (HTML) |
| `GET /api/quote` | Random quote as plain text |
| `GET /api/quote?civ=hre` | Random quote filtered by civ shortname |
| `GET /api/matchup?civ=hre&vs=french` | Random matchup tip for civ vs opponent |
| `GET /api/quotes` | All quotes as JSON |

### Authenticated

| Endpoint | Description |
|----------|-------------|
| `GET /quotes` | Quote management page |
| `POST /quotes` | Add a new quote |
| `POST /quotes/{id}/delete` | Delete a quote |
| `GET /civs` | Civilization management page |

## Civilization Shortnames

The API accepts these shortnames for filtering (based on [aoe4world.com](https://aoe4world.com/explorer)):

| Civ | Shortname | DLC |
|-----|-----------|-----|
| Abbasid Dynasty | `abbasid` | Base Game |
| Chinese | `chinese` | Base Game |
| Delhi Sultanate | `delhi` | Base Game |
| English | `english` | Base Game |
| French | `french` | Base Game |
| Holy Roman Empire | `hre` | Base Game |
| Mongols | `mongols` | Base Game |
| Rus | `rus` | Base Game |
| Malians | `malians` | Anniversary Edition |
| Ottomans | `ottomans` | Anniversary Edition |
| Ayyubids | `ayyubids` | The Sultans Ascend |
| Byzantines | `byzantines` | The Sultans Ascend |
| Japanese | `japanese` | The Sultans Ascend |
| Jeanne d'Arc | `jeannedarc` | The Sultans Ascend |
| Order of the Dragon | `orderofthedragon` | The Sultans Ascend |
| Zhu Xi's Legacy | `zhuxi` | The Sultans Ascend |
| Golden Horde | `goldenhorde` | Dynasties of the East |
| Macedonian Dynasty | `macedonian` | Dynasties of the East |
| Sengoku Daimyo | `sengoku` | Dynasties of the East |
| Tughlaq Dynasty | `tughlaq` | Dynasties of the East |
| House of Lancaster | `lancaster` | Knights of Cross and Rose |
| Knights Templar | `templar` | Knights of Cross and Rose |

## Building and Running

Build with `make build`, then run `./srv/srv`. The server listens on port 8000 by default.

## Running as a systemd service

To run the server as a systemd service:

```bash
# Install the service file
sudo cp srv.service /etc/systemd/system/quotes.service

# Reload systemd and enable the service
sudo systemctl daemon-reload
sudo systemctl enable quotes.service

# Start the service
sudo systemctl start quotes

# Check status
systemctl status quotes

# View logs
journalctl -u quotes -f
```

To restart after code changes:

```bash
make build
sudo systemctl restart quotes
```

## Authorization

This application uses [exe.dev authentication](https://exe.dev/docs/login-with-exe.md).

When proxied through exe.dev, requests include `X-ExeDev-UserID` and `X-ExeDev-Email` headers if the user is authenticated.

The proxy must be set to public for the API to be accessible anonymously:

```bash
ssh exe.dev share set-public <vmname>
```

See [exe.dev proxy documentation](https://exe.dev/docs/proxy.md) for more details.

## Database

This application uses SQLite (`db.sqlite3`). SQL queries are managed with [sqlc](https://sqlc.dev/).

To regenerate query code after modifying `db/queries/*.sql`:

```bash
cd db && go generate
```

Migrations are in `db/migrations/` and run automatically on startup.

## Code Layout

```
├── cmd/srv/          # Main package (binary entrypoint)
├── srv/
│   ├── server.go     # HTTP handlers
│   ├── templates/    # Go HTML templates
│   └── static/       # Static assets
├── db/
│   ├── db.go         # Database setup & migrations
│   ├── migrations/   # SQL migration files
│   ├── queries/      # sqlc query definitions
│   └── dbgen/        # Generated query code
└── srv.service       # systemd unit file
```

## Custom Domain

To use a custom domain, see [exe.dev custom domains documentation](https://exe.dev/docs/custom-domains.md):

1. Add a CNAME record pointing to your exe.dev VM
2. Configure the domain: `ssh exe.dev share domain <vmname> <domain>`
