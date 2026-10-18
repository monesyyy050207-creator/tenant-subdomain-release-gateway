package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/example/tenant-subdomain-release-gateway/internal/tenantrelease"
)

func main() {
	var (
		listenAddr = flag.String("listen", ":8080", "http listen address")
		rootDomain = flag.String("domain", "", "managed root domain, for example example.com")
		bucketName = flag.String("bucket", "", "storage bucket for tenant release artifacts")
	)
	flag.Parse()

	if *rootDomain == "" || *bucketName == "" {
		log.Fatal("-domain and -bucket are required")
	}

	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	service := tenantrelease.NewService(tenantrelease.Config{
		APIKey:     apiKey,
		RootDomain: *rootDomain,
		Bucket:     *bucketName,
	})

	mux := http.NewServeMux()
	mux.Handle("/releases", tenantrelease.NewHandler(service))

	log.Printf("listening on %s", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}
