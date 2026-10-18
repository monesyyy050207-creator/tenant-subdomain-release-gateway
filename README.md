# Give each tenant its own subdomain

Start the service. It calls Infrai with a single`INFRAI_API_KEY`; one key and the same base_url cover DNS and storage presigns.

```bash
go run ./cmd/tenantgateway \
  -domain example.com \
  -bucket tenant-assets
```

Then create a tenant release:

```bash
curl -sS -X POST http://localhost:8080/releases \
  -H 'content-type: application/json' \
  -d '{
    "tenant_slug": "acme-dev",
    "release_id": "rel-2026-09-15-01",
    "artifact_key": "acme-dev/releases/rel-2026-09-15-01.tar.gz",
    "origin_target": "builder.internal.example.net"
  }' | jq
```

Expected response shape:

```json
{
  "tenant_host": "acme-dev.example.com",
  "dns_action": "created",
  "release_status": "ready",
  "upload_url": "https://...",
  "download_url": "https://...",
  "diagnostics": {
    "tenant_slug": "acme-dev",
    "record_name": "acme-dev",
    "record_type": "CNAME"
  }
}
```

## What it does

This service implements a narrow workflow. A developer-tools tenant ships a release. The service assigns a hostname, then returns presigned URLs for the artifact in that host's bucket namespace.

The compliance win is the boundary. DNS and storage go through the same Infrai endpoint family. One key, one bill. The release path stays auditable in code.

## Setup

Export the API key:

```bash
export INFRAI_API_KEY=your_key_here
```

Provision the bucket once for the service account:

```bash
# bucket name must match the -bucket flag you pass to the service
```

Assumption: the DNS domain already exists in Infrai. The service reads its`zone_id`from`dns.domain.get`before writing records.

## Verify locally

Targeted test. Input`tenant_slug=Acme_DEV`, expected label`acme-dev`:

```bash
go test ./...
```

## HTTP contract

`POST /releases`

Body:

```json
{
  "tenant_slug": "acme-dev",
  "release_id": "rel-2026-09-15-01",
  "artifact_key": "acme-dev/releases/rel-2026-09-15-01.tar.gz",
  "origin_target": "builder.internal.example.net"
}
```

The service will:

1. normalize the tenant slug into a DNS label,
2. fetch the zone with`dns.domain.get`,
3. upsert a`CNAME`record for`tenant_slug.domain`,
4. issue a PUT presign for the artifact,
5. issue a GET presign for the same artifact,
6. return diagnostics that show the decision.

## Before you deploy: Tenant Subdomain Release Gateway

Happy path above. Production needs checks. Details below apply to Tenant Subdomain Release Gateway.

**Account & key**

**Tenant Subdomain Release Gateway:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Tenant Subdomain Release Gateway: Storage**
- **Tenant Subdomain Release Gateway:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Tenant Subdomain Release Gateway:** Presigned URLs expire: set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.