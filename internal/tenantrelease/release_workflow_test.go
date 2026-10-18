package tenantrelease

import (
	"context"
	"testing"
)

type fakeClient struct {
	domainCalls  []string
	upsertCalls  []RecordUpsertRequest
	presignCalls []struct {
		bucket string
		key    string
		req    PresignRequest
	}
}

func (f *fakeClient) DomainGet(ctx context.Context, domain string) (DomainGetResponse, error) {
	f.domainCalls = append(f.domainCalls, domain)
	return DomainGetResponse{ZoneID: "zone_123", Domain: domain}, nil
}

func (f *fakeClient) RecordUpsert(ctx context.Context, req RecordUpsertRequest) error {
	f.upsertCalls = append(f.upsertCalls, req)
	return nil
}

func (f *fakeClient) StorageObjectPresign(ctx context.Context, bucket, key string, req PresignRequest) (PresignResponse, error) {
	f.presignCalls = append(f.presignCalls, struct {
		bucket string
		key    string
		req    PresignRequest
	}{bucket: bucket, key: key, req: req})
	return PresignResponse{URL: "https://signed.example/" + key + "?op=" + req.Op}, nil
}

func TestNormalizeTenantLabel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "mixed characters", in: "Acme_DEV", want: "acme-dev"},
		{name: "spaces collapse", in: "  Team   Alpha  ", want: "team-alpha"},
		{name: "fallback", in: "___", want: "tenant"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeTenantLabel(tc.in); got != tc.want {
				t.Fatalf("NormalizeTenantLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPromoteReleaseCreatesTenantDNSAndPresigns(t *testing.T) {
	client := &fakeClient{}
	service := NewServiceWithClient(Config{
		RootDomain: "example.com",
		Bucket:     "tenant-assets",
	}, client)

	result, err := service.PromoteRelease(context.Background(), ReleaseRequest{
		TenantSlug:   "Acme_DEV",
		ReleaseID:    "rel-42",
		ArtifactKey:  "acme-dev/releases/rel-42.tar.gz",
		OriginTarget: "builder.internal.example.net",
	})
	if err != nil {
		t.Fatalf("PromoteRelease returned error: %v", err)
	}

	if result.TenantHost != "acme-dev.example.com" {
		t.Fatalf("TenantHost = %q, want %q", result.TenantHost, "acme-dev.example.com")
	}
	if result.ReleaseStatus != "ready" {
		t.Fatalf("ReleaseStatus = %q, want ready", result.ReleaseStatus)
	}
	if len(client.domainCalls) != 1 || client.domainCalls[0] != "example.com" {
		t.Fatalf("DomainGet calls = %#v, want [example.com]", client.domainCalls)
	}
	if len(client.upsertCalls) != 1 {
		t.Fatalf("RecordUpsert calls = %d, want 1", len(client.upsertCalls))
	}
	upsert := client.upsertCalls[0]
	if upsert.ZoneID != "zone_123" || upsert.RecordType != "CNAME" || upsert.Name != "acme-dev" || upsert.Content != "builder.internal.example.net" {
		t.Fatalf("unexpected upsert call: %#v", upsert)
	}
	if len(client.presignCalls) != 2 {
		t.Fatalf("StorageObjectPresign calls = %d, want 2", len(client.presignCalls))
	}
	if client.presignCalls[0].bucket != "tenant-assets" || client.presignCalls[0].req.Op != "put" {
		t.Fatalf("first presign call = %#v, want put on tenant-assets", client.presignCalls[0])
	}
	if client.presignCalls[1].req.Op != "get" {
		t.Fatalf("second presign call = %#v, want get", client.presignCalls[1])
	}
}
