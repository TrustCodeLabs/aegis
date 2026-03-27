package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	sampleapp "aegis/sample_project/internal/app"
	"aegis/sample_project/internal/httpapi"
)

func main() {
	logger := log.New(os.Stdout, "[sample_project] ", log.LstdFlags|log.Lmicroseconds)

	application, err := sampleapp.New(logger)
	if err != nil {
		logger.Fatalf("failed to build application: %v", err)
	}

	if err := application.GenerateSkillsMarkdown(); err != nil {
		logger.Printf("failed to generate %s: %v", sampleapp.SkillsOutputPath, err)
	}

	port := sampleapp.EnvOrDefault("PORT", sampleapp.DefaultPort)
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           httpapi.New(application).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Printf("%s listening on http://localhost:%s", sampleapp.RuntimeName, port)
	logger.Printf("Try:")
	logger.Printf(`  curl -s http://localhost:%s/ | jq`, port)
	logger.Printf(`  curl -s -X POST http://localhost:%s/api/notes -H 'Content-Type: application/json' -d '{"title":"Minha nota","content":"Olá framework","internal":"somente equipe"}' | jq`, port)
	logger.Printf(`  curl -s http://localhost:%s/api/notes | jq`, port)
	logger.Printf(`  curl -s http://localhost:%s/api/admin/skills -H 'X-Role: admin'`, port)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("server stopped with error: %v", err)
	}
}
