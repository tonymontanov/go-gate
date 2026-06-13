/*
FILE: config_test.go

DESCRIPTION:
Tests for Config defaulting and validation, and for NewClient wiring. Cover:
  - withDefaults fills production endpoints and the default settle;
  - Testnet swaps the default REST/WS endpoints but respects explicit URLs;
  - NewClient succeeds without credentials (public-only) and builds a client.
*/

package gate

import "testing"

func TestConfig_WithDefaults_Production(t *testing.T) {
	var c Config = Config{}.withDefaults()
	if c.Settle != "usdt" {
		t.Fatalf("default settle: got %q want usdt", c.Settle)
	}
	if c.REST.BaseURL != DefaultRestBaseURL {
		t.Fatalf("rest base: got %q want %q", c.REST.BaseURL, DefaultRestBaseURL)
	}
	if c.WS.FuturesURL != DefaultWsFuturesURL {
		t.Fatalf("ws futures: got %q want %q", c.WS.FuturesURL, DefaultWsFuturesURL)
	}
	if c.WS.OptionsURL != DefaultWsOptionsURL {
		t.Fatalf("ws options: got %q want %q", c.WS.OptionsURL, DefaultWsOptionsURL)
	}
	if c.UserAgent != "go-gate/v2" {
		t.Fatalf("user agent: got %q", c.UserAgent)
	}
}

func TestConfig_OptionsURL_TestnetAndExplicit(t *testing.T) {
	// Testnet swaps the default options WS endpoint.
	var c Config = Config{Testnet: true}.withDefaults()
	if c.WS.OptionsURL != TestnetWsOptionsURL {
		t.Fatalf("testnet ws options: got %q want %q", c.WS.OptionsURL, TestnetWsOptionsURL)
	}
	// An explicit options URL is respected even under Testnet.
	c = Config{Testnet: true, WS: WsConfig{OptionsURL: "ws://localhost:9000/options"}}.withDefaults()
	if c.WS.OptionsURL != "ws://localhost:9000/options" {
		t.Fatalf("explicit options url overridden: got %q", c.WS.OptionsURL)
	}
}

func TestConfig_Testnet_SwapsEndpoints(t *testing.T) {
	var c Config = Config{Testnet: true}.withDefaults()
	if c.REST.BaseURL != TestnetRestBaseURL {
		t.Fatalf("testnet rest base: got %q want %q", c.REST.BaseURL, TestnetRestBaseURL)
	}
	if c.WS.FuturesURL != TestnetWsFuturesURL {
		t.Fatalf("testnet ws futures: got %q want %q", c.WS.FuturesURL, TestnetWsFuturesURL)
	}
}

func TestConfig_Testnet_RespectsExplicitURLs(t *testing.T) {
	var c Config = Config{
		Testnet: true,
		REST:    RestConfig{BaseURL: "http://localhost:8080/api/v4"},
		WS:      WsConfig{FuturesURL: "ws://localhost:8080/ws"},
	}.withDefaults()
	if c.REST.BaseURL != "http://localhost:8080/api/v4" {
		t.Fatalf("explicit rest base overridden: got %q", c.REST.BaseURL)
	}
	if c.WS.FuturesURL != "ws://localhost:8080/ws" {
		t.Fatalf("explicit ws url overridden: got %q", c.WS.FuturesURL)
	}
}

func TestNewClient_PublicOnly(t *testing.T) {
	var client *Client
	var err error
	client, err = NewClient(Config{})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client == nil {
		t.Fatalf("client is nil")
	}
	if client.Signer().Enabled() {
		t.Fatalf("signer must be disabled without credentials")
	}
	if err = client.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestNewClient_Signed(t *testing.T) {
	var client *Client
	var err error
	client, err = NewClient(Config{APIKey: "k", SecretKey: "s"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if !client.Signer().Enabled() {
		t.Fatalf("signer must be enabled with credentials")
	}
}
