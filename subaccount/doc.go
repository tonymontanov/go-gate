/*
Package subaccount implements the Gate Sub-Account section (sub-account creation
and API-key management) of the go-gate SDK.

It is named after Gate's own terminology ("Sub-Accounts"). The package owns a back
reference to the root gate.Client (shared signer, REST transport, logger, config)
and exposes a single REST client whose paths live under "/sub_accounts/...". The
Sub-Account section is REST-only: it has NO WebSocket stream, and it is NOT
settle-scoped.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/subaccount"
	)

	client, _ := gate.NewClient(cfg)
	sa := client.SubAccount().(*subaccount.Client)
	subs, err := sa.List(ctx, 0)

GATE SPECIFICS encoded by this package:
  - every endpoint is signed (sub-account and key management are account-level
    write/read operations);
  - an API key carries a list of Permission scopes (name + read_only) and an
    optional IP allow list;
  - a created key's secret is returned ONCE (by CreateKey) and never again — the
    SubAccountKey.Secret field is empty on every subsequent read;
  - Gate epoch-seconds time fields are normalized to epoch milliseconds (...Ms);
  - the numeric Gate state/type enums are surfaced as-is (int64).

CALIBRATION: endpoint paths follow Gate's sub-account docs; the exact
request/response field set (key perms shape, state/type values) is modeled on
those docs — verify field exactness against a live environment.
*/
package subaccount
