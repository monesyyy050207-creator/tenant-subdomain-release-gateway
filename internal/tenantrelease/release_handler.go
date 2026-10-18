package tenantrelease

import (
	"encoding/json"
	"net/http"
)

func NewHandler(service *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ReleaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.TenantSlug == "" || req.ReleaseID == "" || req.ArtifactKey == "" || req.OriginTarget == "" {
			http.Error(w, "missing required field", http.StatusBadRequest)
			return
		}

		result, err := service.PromoteRelease(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
}
