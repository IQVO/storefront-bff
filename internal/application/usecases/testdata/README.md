# Consumer contract testdata

Each `<service>.openapi.yaml` is a byte copy of that producer's `apis/openapi.yaml`
on its `develop` branch. `contracts/pinned.json` lists the pins;
`scripts/harness/pinned_contract_gate.py` (CI job `pinned-contracts`) fails when a
copy drifts from the producer. Refresh = `git -C ../<producer> show
origin/develop:apis/openapi.yaml > <service>.openapi.yaml`, never edit a pin.

The `*.example.json` fixtures are the producer shapes the contract tests decode
through the REAL rest client and gateway (R2 validates each against the pinned
schemas). None of the six producer specs carries whole-response examples for the
routes the BFF calls (checkout carries property-level examples only), so every
fixture is derived from the pinned schemas; producer example backfill is pending.
