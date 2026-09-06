// This file end-to-end tests the MCP surface: the generated Serve*MCP path
// with the server's unary interceptor chain threaded through — the regression
// test for MCP tool calls bypassing protovalidate. It boots the organisation
// MCP service over streamable HTTP exactly as the hybrid server does (an
// MCPServerConfig carrying UnaryInterceptor) and drives it with the real MCP
// client: an invalid create must come back a validation error, a valid one
// must round-trip, and cancelling the context must stop the listener.
//
// Gated like the other live suites (any provider works; gorm shown):
//
//	FREEBUSY_TEST_POSTGRES_DSN="host=... dbname=freebusydb ..." go test ./internal/e2e/ -run TestMCP -v
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oh-tarnished/freebusy/internal"
	"github.com/oh-tarnished/freebusy/internal/database"
	"github.com/oh-tarnished/freebusy/protobuf/generated/go/promocode/v1/promocodepbv1"
	"github.com/the-protobuf-project/runtime-go/grpc"
)

func TestMCP_ValidationAndLifecycle_Gorm(t *testing.T) {
	db := openTestGorm(t)
	_, svc, err := internal.NewGRPCServer(&database.Connection{PgSQLConn: db, Provider: database.ProviderGorm})
	if err != nil {
		t.Fatalf("assemble service: %v", err)
	}

	validate, err := grpc.NewValidationInterceptor()
	if err != nil {
		t.Fatalf("build validation interceptor: %v", err)
	}

	// Reserve a port the way the hybrid server binds one per MCP service.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	cfg := &grpc.MCPServerConfig{
		Name:             "freebusy-e2e",
		Version:          "0.0.0",
		Addr:             addr,
		UnaryInterceptor: validate, // what buildMCPConfigForPort threads through
	}
	go func() { served <- promocodepbv1.ServePromoCodeServiceMCP(ctx, svc, cfg) }()

	endpoint := fmt.Sprintf("http://%s%s", addr, promocodepbv1.PromoCodeServiceMCPDefaultBasePath)
	session := dialMCP(t, endpoint)

	// Invalid create — the interceptor must reject it before the handler runs
	// (this used to panic or write an unvalidated row).
	//
	// Driven through PromoCodeService since OrganisationService was retired to
	// RFC 4519; the subject is the MCP surface and the interceptor, not the
	// particular service.
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "promo_code_service-create_promo_code_v1",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("invalid create transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("invalid create was accepted: %v", res.Content)
	}
	if text := toolText(res); !strings.Contains(text, "promo_code") {
		t.Fatalf("rejection does not read as a validation error: %q", text)
	}

	// Valid create round-trips, and the row is cleaned up over the same surface.
	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "promo_code_service-create_promo_code_v1",
		Arguments: map[string]any{
			"promo_code": map[string]any{
				"code":     fmt.Sprintf("MCPE2E%d", time.Now().UnixNano()%1_000_000_000),
				"discount": map[string]any{"percent_off": 10},
			},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("valid create failed: err=%v content=%v", err, res.Content)
	}
	name := extractJSONField(toolText(res), "name")
	if !strings.HasPrefix(name, "promoCodes/") {
		t.Fatalf("created name = %q", name)
	}
	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "promo_code_service-delete_promo_code_v1",
		Arguments: map[string]any{"name": name, "force": true},
	})
	if err != nil || res.IsError {
		t.Fatalf("delete failed: err=%v content=%v", err, res.Content)
	}
	_ = session.Close()

	// Context cancellation must stop the listener (the old StartServer blocked
	// in ListenAndServe forever).
	cancel()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("ServePromoCodeServiceMCP did not return after context cancellation")
	}
}

// dialMCP connects the real MCP client over streamable HTTP, retrying briefly
// while the server goroutine binds its listener.
func dialMCP(t *testing.T, endpoint string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0.0.0"}, nil)
	deadline := time.Now().Add(5 * time.Second)
	for {
		session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
		if err == nil {
			t.Cleanup(func() { _ = session.Close() })
			return session
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect MCP %s: %v", endpoint, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// toolText flattens a tool result's text content.
func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// extractJSONField pulls a top-level string field out of a JSON text blob
// without committing to the full response shape.
func extractJSONField(text, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return ""
	}
	s, _ := m[field].(string)
	return s
}
