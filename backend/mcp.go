package main

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// A stateless Streamable HTTP MCP endpoint. POST returns JSON (permitted by
// the transport), notifications return 202, and server-push GET is unsupported.
// No session IDs are issued: every request authenticates its own bearer key.
var mcpVersions = []string{"2025-03-26", "2025-06-18", "2025-11-25"}

func (a *API) handleMCP(w http.ResponseWriter, r *http.Request, key AgentKey) {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "https" && u.Scheme != "http") {
			writeErr(w, 403, "origin not allowed")
			return
		}
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeErr(w, 405, "MCP server supports stateless POST requests")
		return
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		writeErr(w, 415, "application/json required")
		return
	}
	accept := r.Header.Get("Accept")
	if accept != "" && !strings.Contains(accept, "application/json") && !strings.Contains(accept, "*/*") {
		writeErr(w, 406, "Accept application/json required")
		return
	}
	if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !slices.Contains(mcpVersions, v) {
		writeErr(w, 400, "unsupported MCP protocol version")
		return
	}
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	rpcError := func(status, code int, message string) {
		id := request.ID
		if len(id) == 0 {
			id = json.RawMessage(`null`)
		}
		writeJSON(w, status, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
	}
	if decodeJSON(w, r, &request, 32<<10) != nil {
		request.ID = nil
		rpcError(400, -32700, "Parse error")
		return
	}
	if request.JSONRPC != "2.0" || request.Method == "" {
		request.ID = nil
		rpcError(400, -32600, "Invalid Request")
		return
	}
	if len(request.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var id any
	if json.Unmarshal(request.ID, &id) != nil {
		request.ID = nil
		rpcError(400, -32600, "Invalid request ID")
		return
	}
	switch id.(type) {
	case string, float64:
	default:
		request.ID = nil
		rpcError(400, -32600, "Invalid request ID")
		return
	}
	result := func(v any) { writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": v}) }
	switch request.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string          `json:"protocolVersion"`
			Capabilities    json.RawMessage `json:"capabilities"`
			ClientInfo      json.RawMessage `json:"clientInfo"`
		}
		if json.Unmarshal(request.Params, &params) != nil || params.ProtocolVersion == "" {
			rpcError(200, -32602, "protocolVersion is required")
			return
		}
		version := params.ProtocolVersion
		if !slices.Contains(mcpVersions, version) {
			version = mcpVersions[len(mcpVersions)-1]
		}
		result(map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]bool{"listChanged": false}}, "serverInfo": map[string]string{"name": "xunshu", "version": "2.0.0"}, "instructions": agentPurpose})
	case "ping":
		result(map[string]any{})
	case "tools/list":
		if len(request.Params) > 0 {
			var params struct {
				Cursor string `json:"cursor"`
			}
			if json.Unmarshal(request.Params, &params) != nil || params.Cursor != "" {
				rpcError(200, -32602, "Invalid cursor")
				return
			}
		}
		result(map[string]any{"tools": agentTools()})
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Meta      json.RawMessage `json:"_meta,omitempty"`
		}
		if json.Unmarshal(request.Params, &params) != nil || params.Name == "" {
			rpcError(200, -32602, "Tool name is required")
			return
		}
		known := false
		for _, tool := range agentTools() {
			if tool.Name == params.Name {
				known = true
				break
			}
		}
		if !known {
			rpcError(200, -32602, "Unknown tool")
			return
		}
		if len(params.Arguments) == 0 {
			params.Arguments = json.RawMessage(`{}`)
		}
		text := a.executeAgentQuery(r.Context(), key, params.Name, params.Arguments)
		var data map[string]any
		_ = json.Unmarshal([]byte(text), &data)
		result(map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "structuredContent": data, "isError": data["error"] != nil})
	default:
		rpcError(200, -32601, "Method not found")
	}
}
