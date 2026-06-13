/*
FILE: examples/subaccount/main.go

DESCRIPTION:
A minimal runnable example for the go-gate sub-account section (sub-account +
API-key management). Every endpoint in this section is signed, so the example is
a no-op without credentials. With GATE_API_KEY / GATE_API_SECRET set it performs
read-only signed calls (list sub-accounts, then list one sub-account's API keys).
It does NOT create sub-accounts/keys or lock/unlock anything: those are
account-changing actions that should be opt-in.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/subaccount

NOTE: the sub-account section is calibration-pending (endpoint/field exactness is
modeled on Gate's sub-account docs); verify against a live environment.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/subaccount"
	satypes "github.com/tonymontanov/go-gate/v2/subaccount/types"
)

func main() {
	var client *gate.Client
	var err error
	client, err = gate.NewClient(gate.Config{
		APIKey:    os.Getenv("GATE_API_KEY"),
		SecretKey: os.Getenv("GATE_API_SECRET"),
	})
	if err != nil {
		log.Fatalf("new client: %v", err)
	}
	defer func() { _ = client.Close() }()

	var sa *subaccount.Client = client.SubAccount().(*subaccount.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Every sub-account endpoint is signed: without creds there is nothing to do.
	if os.Getenv("GATE_API_KEY") == "" || os.Getenv("GATE_API_SECRET") == "" {
		log.Printf("no credentials set — sub-account endpoints are all signed; nothing to demo")
		return
	}

	// --- private: list sub-accounts ---
	var subs []satypes.SubAccount
	subs, err = sa.List(ctx, "")
	if err != nil {
		log.Fatalf("list sub-accounts: %v", err)
	}
	log.Printf("sub-accounts: %d", len(subs))
	if len(subs) == 0 {
		log.Printf("no sub-accounts — skipping key demo")
		return
	}
	var first satypes.SubAccount = subs[0]
	log.Printf("first sub-account: user=%s login=%s state=%d", first.UserID, first.LoginName, first.State)

	// --- private: list the first sub-account's API keys (read-only) ---
	var keys []satypes.SubAccountKey
	keys, err = sa.ListKeys(ctx, first.UserID)
	if err != nil {
		log.Printf("list keys: %v", err)
	} else {
		log.Printf("API keys for %s: %d", first.UserID, len(keys))
		var i int
		for i = 0; i < len(keys); i++ {
			log.Printf("  key %s (perms=%d, ip_whitelist=%d)",
				keys[i].Key, len(keys[i].Perms), len(keys[i].IPWhitelist))
		}
	}
}
