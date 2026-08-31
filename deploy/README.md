# Deploy KeiRouter

KeiRouter can run as a local binary, a single Docker container with SQLite, or a
container plus Postgres for team/VPS deployments.

## Quick Start (Pre-built Docker Image)

Pull and run the latest public image from GitHub Container Registry:

```bash
docker pull ghcr.io/zsgworks/keirouter:latest

# Run with docker compose (recommended)
docker compose up -d
```

Or run directly with Docker:

```bash
docker run -d \
  --name keirouter \
  -p 20180:20180 \
  -v keirouter-data:/data \
  ghcr.io/zsgworks/keirouter:latest
```

Available tags:
- `ghcr.io/zsgworks/keirouter:latest` — latest stable from `main`
- `ghcr.io/zsgworks/keirouter:1.2.3` — specific version
- `ghcr.io/zsgworks/keirouter:sha-abc1234` — pinned to a commit

## Local Development (One-Liner)

Run this single command — it clones, installs deps, and starts everything:

```bash
curl -fsSL https://raw.githubusercontent.com/ZSGWorks/keirouter/main/scripts/quickstart.sh | bash
```

No `.env`, no config, no manual steps. It will:
- Check Go 1.24+ and Node.js 20+ are installed
- Clone the repo to `~/keirouter` (or use existing checkout)
- Install frontend dependencies (npm ci)
- Download Go modules
- Start backend (:20180) and dashboard (:5180) with hot reload

Dashboard: http://localhost:5180 (password: `keirouter`)

> **Already have the repo?** Just run `make setup` from the project root.

## Local Install (Binary)

Build and install the binary + dashboard assets system-wide:

```bash
curl -fsSL https://raw.githubusercontent.com/ZSGWorks/keirouter/main/scripts/install.sh | bash
keirouter
```

If you prefer Docker and do not want Go/Node.js on the machine:

```bash
curl -fsSL https://raw.githubusercontent.com/ZSGWorks/keirouter/main/scripts/install.sh | bash -s -- --docker
```

## VPS Deployment Guide

### Option 1: VPS with Docker Compose (SQLite - Default)

This is the simplest way to get KeiRouter running on a clean VPS (Ubuntu/Debian).

**1. Clone the repository**
```bash
git clone https://github.com/ZSGWorks/keirouter.git
cd keirouter
```

**2. Configure Environment Variables**
```bash
cp .env.example .env
```
Open `.env` with your favorite editor (e.g., `nano .env`). You **must** generate a secure 32-byte master key and set it as `KEIROUTER_MASTER_KEY`:
```bash
# Generate a key locally:
openssl rand -base64 32
```
Paste the generated key into your `.env` file.

**3. Start the Deployment**
```bash
docker compose up -d --build
```

**4. View Logs**
```bash
docker compose logs -f keirouter
```

**5. Access the Dashboard**
By default, KeiRouter will be available at `http://YOUR_VPS_IP:20180`. KeiRouter automatically trusts `X-Forwarded-Proto` from TLS-terminating reverse proxies by default, so cookies will be marked Secure only when you actually serve HTTPS through nginx/Caddy/Traefik. The dashboard session cookie remains usable over plain HTTP for local-only deployments (see issue #56).

It is highly recommended to put KeiRouter behind a reverse proxy like **Nginx**, **Caddy**, or **Traefik** to secure it with HTTPS and a custom domain.

### Option 2: VPS with Postgres

If you are deploying for a team or expecting high traffic, you can use Postgres instead of SQLite.

**1. Prepare `.env`**
Follow the steps above to create your `.env` file, but make sure you also set the database password:
```env
POSTGRES_PASSWORD=your_secure_postgres_password
```

**2. Start the Deployment**
Use the override compose file to start both KeiRouter and Postgres:
```bash
docker compose -f compose.yaml -f compose.postgres.yaml up -d --build
```
*Note: The app container still stores runtime secrets and generated files in a Docker volume mounted at `/data`, while request/account data is stored in the Postgres database.*

## Coolify Deployment Guide

Deploying KeiRouter on [Coolify](https://coolify.io/) is highly recommended as it automates SSL/TLS certificates and reverse proxy configuration.

### Deployment Steps

1. **Create Postgres**: In your Coolify project and environment, add a **PostgreSQL** resource first. Copy its internal connection URL from the resource's Connection tab.
2. **Create KeiRouter Resource**: Add a **Git Repository** resource and enter:
    - **Repository URL**: `https://github.com/ZSGWorks/keirouter`
    - **Branch**: `main`
3. **Build Pack**: Select **Docker Compose**. Set **Docker Compose Location** to `compose.coolify-postgres.yaml`.
4. **Configuration**:
    - **Domains**: Enter your custom domain with its container target port (e.g., `https://keirouter.yourdomain.com:20180`). Coolify uses the suffix to route to port `20180`; it does not expose port `20180` publicly.
5. **Environment Variables**:
    Navigate to the Environment Variables tab in Coolify and add the following variables (Switch to Developer view to edit as text):
    ```env
    KEIROUTER_BIND_LOOPBACK_ONLY=false
   # Coolify terminates TLS and forwards via X-Forwarded-Proto; trusting it
   # (the default) makes the dashboard session cookie Secure over HTTPS.
    KEIROUTER_TRUST_FORWARDED_HEADERS=true
    # Generate a 32-byte base64 key locally and paste it here:
    KEIROUTER_MASTER_KEY=<your_generated_master_key>
    # Use the Coolify Postgres resource's internal connection URL. Add
    # sslmode=require when the resource enforces TLS.
    KEIROUTER_DATABASE__DSN=postgres://USER:PASSWORD@HOST:5432/DB?sslmode=disable
    KEIROUTER_LOG_FORMAT=json
    ```
6. **Persistent Storage**:
    `compose.coolify-postgres.yaml` declares `keirouter-data` at `/data` for runtime secrets. Database data stays in the separate Coolify Postgres resource.
7. **Network**: The Compose file joins Coolify's external `coolify` network so it can resolve the Postgres resource's internal hostname. Change that network name in the file if your Coolify host uses a different one.
8. **Deploy**: Click **Deploy**. Coolify builds `deploy/Dockerfile` from this repository before starting KeiRouter. Set `KEIROUTER_VERSION` to stamp a release version; otherwise the Coolify-injected `SOURCE_COMMIT` is shown in the dashboard.

### Using External/Managed Postgres on Coolify

`compose.coolify-postgres.yaml` already sets the Postgres driver. Supply its required DSN in the Coolify environment:

```env
KEIROUTER_DATABASE__DSN=postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require
```
*(Replace `USER`, `PASSWORD`, `HOST`, and `DB` with your Postgres credentials).*

## Updates

```bash
git pull
docker compose up -d --build
```

For the local source installer, rerun the install command.

## Security Notes

- Keep `KEIROUTER_MASTER_KEY` stable and backed up.
- Use HTTPS when exposing the dashboard outside localhost.
- The dashboard has session auth, but production deployments should still sit
  behind a reverse proxy, firewall, or platform access control.
- The default dashboard password is `keirouter`; change it on first login.
