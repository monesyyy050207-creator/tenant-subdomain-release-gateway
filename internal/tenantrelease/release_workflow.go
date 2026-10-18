package tenantrelease

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type Config struct {
	APIKey     string
	RootDomain string
	Bucket     string
}

type Service struct {
	client     apiClient
	rootDomain string
	bucket     string
}

type apiClient interface {
	DomainGet(ctx context.Context, domain string) (DomainGetResponse, error)
	RecordUpsert(ctx context.Context, req RecordUpsertRequest) error
	StorageObjectPresign(ctx context.Context, bucket, key string, req PresignRequest) (PresignResponse, error)
}

type ReleaseRequest struct {
	TenantSlug   string `json:"tenant_slug"`
	ReleaseID    string `json:"release_id"`
	ArtifactKey  string `json:"artifact_key"`
	OriginTarget string `json:"origin_target"`
}

type ReleaseDiagnostics struct {
	TenantSlug string `json:"tenant_slug"`
	RecordName string `json:"record_name"`
	RecordType string `json:"record_type"`
}

type ReleaseResult struct {
	TenantHost    string             `json:"tenant_host"`
	DNSAction     string             `json:"dns_action"`
	ReleaseStatus string             `json:"release_status"`
	UploadURL     string             `json:"upload_url"`
	DownloadURL   string             `json:"download_url"`
	Diagnostics   ReleaseDiagnostics `json:"diagnostics"`
}

func NewService(cfg Config) *Service {
	return &Service{
		client:     NewClient(cfg.APIKey),
		rootDomain: cfg.RootDomain,
		bucket:     cfg.Bucket,
	}
}

func NewServiceWithClient(cfg Config, client apiClient) *Service {
	return &Service{
		client:     client,
		rootDomain: cfg.RootDomain,
		bucket:     cfg.Bucket,
	}
}

func (s *Service) PromoteRelease(ctx context.Context, req ReleaseRequest) (ReleaseResult, error) {
	label := NormalizeTenantLabel(req.TenantSlug)
	zone, err := s.client.DomainGet(ctx, s.rootDomain)
	if err != nil {
		return ReleaseResult{}, err
	}

	recordName := label
	if err := s.client.RecordUpsert(ctx, RecordUpsertRequest{
		ZoneID:     zone.ZoneID,
		RecordType: "CNAME",
		Name:       recordName,
		Content:    req.OriginTarget,
		TTL:        60,
		Metadata: map[string]any{
			"release_id": req.ReleaseID,
		},
	}); err != nil {
		return ReleaseResult{}, err
	}

	putPresign, err := s.client.StorageObjectPresign(ctx, s.bucket, req.ArtifactKey, PresignRequest{
		Op:             "put",
		ExpiresSeconds: 900,
		ContentType:    "application/gzip",
		MaxBytes:       200 << 20,
		IdempotencyKey: req.ReleaseID + ":put",
	})
	if err != nil {
		return ReleaseResult{}, err
	}

	getPresign, err := s.client.StorageObjectPresign(ctx, s.bucket, req.ArtifactKey, PresignRequest{
		Op:                  "get",
		ExpiresSeconds:      900,
		ResponseDisposition: fmt.Sprintf("attachment; filename=%q", label+"-"+req.ReleaseID+".tar.gz"),
		IdempotencyKey:      req.ReleaseID + ":get",
	})
	if err != nil {
		return ReleaseResult{}, err
	}

	return ReleaseResult{
		TenantHost:    label + "." + s.rootDomain,
		DNSAction:     "created",
		ReleaseStatus: "ready",
		UploadURL:     putPresign.URL,
		DownloadURL:   getPresign.URL,
		Diagnostics: ReleaseDiagnostics{
			TenantSlug: label,
			RecordName: recordName,
			RecordType: "CNAME",
		},
	}, nil
}

var invalidLabelChars = regexp.MustCompile(`[^a-z0-9-]+`)
var repeatedHyphen = regexp.MustCompile(`-+`)

func NormalizeTenantLabel(in string) string {
	v := strings.ToLower(strings.TrimSpace(in))
	v = strings.ReplaceAll(v, "_", "-")
	v = invalidLabelChars.ReplaceAllString(v, "-")
	v = repeatedHyphen.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return "tenant"
	}
	if len(v) > 63 {
		v = strings.Trim(v[:63], "-")
		if v == "" {
			return "tenant"
		}
	}
	return v
}
