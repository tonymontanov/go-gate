/*
FILE: subaccount/subaccount_test.go

DESCRIPTION:
Contract tests for the subaccount client against httptest-served Gate JSON.
newSubAccountTestClient points the parent gate.Client's REST transport at an
httptest.Server so the client issues real HTTP requests with no network. The
tests pin: the List parse, the Create request-body + parse, the CreateKey
request-body (with nested perms) + parse (secret), the Lock path/method, the
DeleteKey path/method, and a Gate {label,message} error surfacing as *gate.Error.
*/

package subaccount

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/subaccount/types"
)

func newSubAccountTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	var parent *gate.Client
	var err error
	parent, err = gate.NewClient(gate.Config{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		REST:      gate.RestConfig{BaseURL: baseURL},
	})
	if err != nil {
		t.Fatalf("gate.NewClient: %v", err)
	}
	return NewClient(parent)
}

// decodeBody reads and JSON-decodes the request body into a generic map.
func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var raw []byte
	raw, _ = io.ReadAll(r.Body)
	var m map[string]any = map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("decode body: %v (raw=%s)", err, string(raw))
		}
	}
	return m
}

func TestList_Parse(t *testing.T) {
	var gotPath, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("KEY")
		_, _ = io.WriteString(w, `[{"user_id":10001,"login_name":"sub1","remark":"team A",
			"email":"a@example.com","state":1,"type":1,"create_time":1546905927}]`)
	}))
	defer srv.Close()

	var sa *Client = newSubAccountTestClient(t, srv.URL)
	var subs []types.SubAccount
	var err error
	subs, err = sa.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotPath != "/sub_accounts" {
		t.Fatalf("path: %q", gotPath)
	}
	if gotKey == "" {
		t.Fatalf("private call should be signed (KEY header missing)")
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 sub-account, got %d", len(subs))
	}
	if subs[0].UserID != "10001" || subs[0].LoginName != "sub1" || subs[0].State != 1 {
		t.Fatalf("sub: %+v", subs[0])
	}
	if subs[0].CreatedAtMs != 1546905927000 {
		t.Fatalf("created: %d", subs[0].CreatedAtMs)
	}
}

func TestCreate_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"user_id":10002,"login_name":"sub2","remark":"team B",
			"state":1,"type":1,"create_time":1700000000}`)
	}))
	defer srv.Close()

	var sa *Client = newSubAccountTestClient(t, srv.URL)
	var sub types.SubAccount
	var err error
	sub, err = sa.Create(context.Background(), types.CreateSubAccountRequest{
		LoginName: "sub2",
		Remark:    "team B",
		Email:     "b@example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/sub_accounts" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["login_name"] != "sub2" || gotBody["remark"] != "team B" || gotBody["email"] != "b@example.com" {
		t.Fatalf("body: %+v", gotBody)
	}
	if sub.UserID != "10002" || sub.LoginName != "sub2" || sub.CreatedAtMs != 1700000000000 {
		t.Fatalf("parsed sub: %+v", sub)
	}
}

func TestCreateKey_BodyNestedPermsAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"user_id":"10001","name":"bot","key":"PUBLICKEY","secret":"SECRETKEY",
			"perms":[{"name":"spot","read_only":false},{"name":"wallet","read_only":true}],
			"ip_whitelist":["1.2.3.4"],"state":1,"created_at":1700000000,"updated_at":1700000001}`)
	}))
	defer srv.Close()

	var sa *Client = newSubAccountTestClient(t, srv.URL)
	var key types.SubAccountKey
	var err error
	key, err = sa.CreateKey(context.Background(), "10001", types.CreateKeyRequest{
		Name: "bot",
		Perms: []types.Permission{
			{Name: "spot", ReadOnly: false},
			{Name: "wallet", ReadOnly: true},
		},
		IPWhitelist: []string{"1.2.3.4"},
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/sub_accounts/10001/keys" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["name"] != "bot" {
		t.Fatalf("body name: %+v", gotBody)
	}
	// Nested perms: a JSON array of {name, read_only} objects.
	var perms []any
	var ok bool
	perms, ok = gotBody["perms"].([]any)
	if !ok || len(perms) != 2 {
		t.Fatalf("perms not a 2-element array: %+v", gotBody["perms"])
	}
	var first map[string]any = perms[0].(map[string]any)
	if first["name"] != "spot" || first["read_only"] != false {
		t.Fatalf("perm[0]: %+v", first)
	}
	var second map[string]any = perms[1].(map[string]any)
	if second["name"] != "wallet" || second["read_only"] != true {
		t.Fatalf("perm[1]: %+v", second)
	}
	if key.Key != "PUBLICKEY" || key.Secret != "SECRETKEY" || key.UserID != "10001" {
		t.Fatalf("parsed key: %+v", key)
	}
	if len(key.Perms) != 2 || key.Perms[1].Name != "wallet" || !key.Perms[1].ReadOnly {
		t.Fatalf("parsed perms: %+v", key.Perms)
	}
	if len(key.IPWhitelist) != 1 || key.IPWhitelist[0] != "1.2.3.4" {
		t.Fatalf("parsed ip whitelist: %+v", key.IPWhitelist)
	}
}

func TestLock_PathAndMethod(t *testing.T) {
	var gotPath, gotMethod string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	var sa *Client = newSubAccountTestClient(t, srv.URL)
	var err error
	err = sa.Lock(context.Background(), "10001")
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/sub_accounts/10001/lock" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
}

func TestDeleteKey_PathAndMethod(t *testing.T) {
	var gotPath, gotMethod string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	var sa *Client = newSubAccountTestClient(t, srv.URL)
	var err error
	err = sa.DeleteKey(context.Background(), "10001", "PUBLICKEY")
	if err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/sub_accounts/10001/keys/PUBLICKEY" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
}

func TestSubAccount_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"login_name exists"}`)
	}))
	defer srv.Close()

	var sa *Client = newSubAccountTestClient(t, srv.URL)
	var err error
	_, err = sa.Create(context.Background(), types.CreateSubAccountRequest{LoginName: "dup"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
