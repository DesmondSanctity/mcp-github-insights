package main

import (
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"mcp-github-insights/tools"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := "0.0.0.0:" + port

	s := server.NewMCPServer(
		"github-insights",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)
	tools.Register(s)

	httpServer := server.NewStreamableHTTPServer(s)
	log.Printf("MCP GitHub Insights server listening on %s (endpoint: /mcp)", addr)
	if err := httpServer.Start(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
