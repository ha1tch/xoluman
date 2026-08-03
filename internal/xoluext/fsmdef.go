// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package xoluext

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	xclient "github.com/ha1tch/xolu/pkg/client"

	"github.com/ha1tch/xoluman/internal/connstore"
)

// This file exists because xolu/pkg/client has no write methods for FSM
// definitions yet — CreateMachineDef, ReplaceMachineDef, DeleteMachineDef,
// ValidateMachineDef. Server-side support already exists
// (POST/GET/PUT/DELETE /api/v2/fsm/def, POST /api/v2/fsm/def/validate,
// verified directly against pkg/server/v2_fsm_def_handlers.go — not
// assumed from docs) and has been requested from the xolu team
// (docs/xolu-requests-fsm-def.md, tracked as T-13). Building against
// the documented REST endpoints directly in the meantime rather than
// waiting idle — this is xoluman consuming xolu's own public API, not
// xoluman modifying xolu (see docs/KNOWN_ISSUES.md's recorded
// decision on that boundary; this file lives entirely in xoluman's own
// repository).
//
// Delete this file and switch every caller to the real client methods
// once T-13 lands — the request document's proposed signatures were
// written to match this file's shape exactly, so that swap should be
// close to mechanical.
//
// The auth-header and v2-URL-building logic below duplicates a few
// lines of what xolu/pkg/client's authHeader()/buildURLv2() do
// internally — both are unexported, so there's no way to reuse them
// from outside the package. Kept intentionally minimal.

// MachineDefCreateResult is returned by CreateMachineDef.
type MachineDefCreateResult struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	CreatedAt string          `json:"created_at"`
	Analysis  json.RawMessage `json:"analysis"`
}

// MachineDefReplaceResult is returned by ReplaceMachineDef.
type MachineDefReplaceResult struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Analysis json.RawMessage `json:"analysis"`
}

// MachineDefValidation is returned by ValidateMachineDef. The endpoint
// always responds 200 regardless of validity — Valid distinguishes a
// rejected spec from an accepted one; a non-nil error from this
// function means a transport/decode failure, not an invalid spec.
type MachineDefValidation struct {
	Valid    bool                        `json:"valid"`
	Analysis json.RawMessage             `json:"analysis,omitempty"`
	Errors   []MachineDefValidationError `json:"errors,omitempty"`
}

// MachineDefValidationError is one entry in MachineDefValidation.Errors.
type MachineDefValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// fsmDefAuthHeader mirrors xolu/pkg/client's authHeader(): all three
// authenticated modes send the same "Bearer <token>" shape, differing
// only in the token's content and how the server validates it.
func fsmDefAuthHeader(conn connstore.Connection) string {
	if conn.AuthMode == connstore.AuthNone || conn.Token == "" {
		return ""
	}
	return "Bearer " + conn.Token
}

// fsmDefURL mirrors xolu/pkg/client's buildURLv2(): tenant-scoped when
// a tenant is configured, root /api/v2 otherwise.
func fsmDefURL(conn connstore.Connection, path string) string {
	if conn.Tenant != "" {
		return fmt.Sprintf("%s/api/v2/tenant/%s%s", conn.BaseURL, conn.Tenant, path)
	}
	return fmt.Sprintf("%s/api/v2%s", conn.BaseURL, path)
}

// fsmDefDo executes an authenticated request against conn's xolu
// instance and decodes a JSON response into result (skipped if result
// is nil, for DELETE's 204 No Content). Non-2xx responses are decoded
// as xolu's structured error envelope (matching client.Error's own two
// supported shapes) and returned as *xclient.Error, so callers get the
// same error type regardless of whether a request went through the
// official client or this temporary path.
func fsmDefDo(ctx context.Context, method, url string, authHeader string, body any, result any) error {
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var respBody []byte
	if resp.ContentLength != 0 {
		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %w", err)
		}
	}

	if resp.StatusCode >= 400 {
		return decodeFSMDefError(resp.StatusCode, respBody)
	}
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// decodeFSMDefError builds a *xclient.Error from a non-2xx response,
// trying xolu's structured {"error":{"code","message"}} envelope first
// and falling back to the raw body as the message — the same two
// shapes client.Client.decodeResponse itself handles.
func decodeFSMDefError(status int, body []byte) error {
	xoluErr := &xclient.Error{HTTPStatus: status, StatusCode: status}
	if len(body) > 0 {
		xoluErr.Detail = json.RawMessage(body)
	}

	var structured struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &structured); err == nil && structured.Error.Code != "" {
		xoluErr.Code = structured.Error.Code
		xoluErr.Message = structured.Error.Message
		return xoluErr
	}
	xoluErr.Message = string(body)
	return xoluErr
}

// CreateMachineDef creates a new FSM definition.
//
// Hits POST /api/v2/fsm/def with spec as the raw request body.
func CreateMachineDef(ctx context.Context, conn connstore.Connection, spec xclient.MachineSpec) (*MachineDefCreateResult, error) {
	var result MachineDefCreateResult
	url := fsmDefURL(conn, "/fsm/def")
	if err := fsmDefDo(ctx, http.MethodPost, url, fsmDefAuthHeader(conn), spec, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ReplaceMachineDef replaces an existing FSM definition's spec.
//
// Per the endpoint's own route comment ("replace a definition (future
// machines only)"), this does not retroactively affect already-created
// machine instances — only what a new machine gets when created against
// this definition afterward.
//
// Hits PUT /api/v2/fsm/def/{id} with spec as the raw request body.
func ReplaceMachineDef(ctx context.Context, conn connstore.Connection, id int64, spec xclient.MachineSpec) (*MachineDefReplaceResult, error) {
	var result MachineDefReplaceResult
	url := fsmDefURL(conn, fmt.Sprintf("/fsm/def/%d", id))
	if err := fsmDefDo(ctx, http.MethodPut, url, fsmDefAuthHeader(conn), spec, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteMachineDef deletes an FSM definition. Per the endpoint's own
// route comment, always permitted — no check against existing machines
// referencing it.
//
// Hits DELETE /api/v2/fsm/def/{id}.
func DeleteMachineDef(ctx context.Context, conn connstore.Connection, id int64) error {
	url := fsmDefURL(conn, fmt.Sprintf("/fsm/def/%d", id))
	return fsmDefDo(ctx, http.MethodDelete, url, fsmDefAuthHeader(conn), nil, nil)
}

// ValidateMachineDef validates a spec against xolu's structural
// analysis without persisting it. Always returns a *MachineDefValidation
// on a successful call — Valid distinguishes acceptance from rejection;
// a non-nil error means the request itself failed, not that the spec
// was invalid.
//
// Hits POST /api/v2/fsm/def/validate with spec as the raw request body.
func ValidateMachineDef(ctx context.Context, conn connstore.Connection, spec xclient.MachineSpec) (*MachineDefValidation, error) {
	var result MachineDefValidation
	url := fsmDefURL(conn, "/fsm/def/validate")
	if err := fsmDefDo(ctx, http.MethodPost, url, fsmDefAuthHeader(conn), spec, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
