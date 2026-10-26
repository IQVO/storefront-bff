package usecases

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

type tierCharge struct {
	Tier     string          `json:"tier"`
	Charge   json.RawMessage `json:"charge"`
	FreeOver json.RawMessage `json:"freeOver,omitempty"`
}

// chargesByTier reads the current delivery charge schedule; any failure
// yields no charges (the dates still render).
func (g *Gateway) chargesByTier(ctx context.Context) map[string]tierCharge {
	reply, err := g.call(ctx, Actor{}, composition.Call{
		Service: composition.Pricing, Method: "GET", Path: "/delivery-charge-schedules/current",
	})
	if err != nil || !reply.OK() {
		return nil
	}
	var sched struct {
		Tiers []tierCharge `json:"tiers"`
	}
	if json.Unmarshal(reply.Body, &sched) != nil {
		return nil
	}
	out := make(map[string]tierCharge, len(sched.Tiers))
	for _, t := range sched.Tiers {
		out[t.Tier] = t
	}
	return out
}

type quotedOffer struct {
	State  string `json:"state"`
	Tier   string `json:"tier"`
	Window struct {
		Earliest string `json:"earliest"`
		Latest   string `json:"latest"`
	} `json:"window"`
	ExpiresAt string `json:"expiresAt"`
}

type dateTier struct {
	Tier      string          `json:"tier"`
	Earliest  string          `json:"deliverByEarliest"`
	Latest    string          `json:"deliverByLatest"`
	ExpiresAt string          `json:"offerExpiresAt"`
	Charge    json.RawMessage `json:"charge,omitempty"`
	FreeOver  json.RawMessage `json:"freeOver,omitempty"`
}

// composeDates turns a delivery-promise quote into the product page answer:
// one entry per quoted tier with its window and charge, plus the tiers the
// network cannot serve.
func composeDates(prefix string, quote []byte, charges map[string]tierCharge) ([]byte, error) {
	var q struct {
		Offers      []quotedOffer     `json:"offers"`
		Unavailable []json.RawMessage `json:"unavailable"`
	}
	if err := json.Unmarshal(quote, &q); err != nil {
		return nil, err
	}
	tiers := make([]dateTier, 0, len(q.Offers))
	for _, o := range q.Offers {
		if o.State != "QUOTED" {
			continue
		}
		d := dateTier{Tier: o.Tier, Earliest: o.Window.Earliest, Latest: o.Window.Latest, ExpiresAt: o.ExpiresAt}
		if c, ok := charges[o.Tier]; ok {
			d.Charge, d.FreeOver = c.Charge, c.FreeOver
		}
		tiers = append(tiers, d)
	}
	if q.Unavailable == nil {
		q.Unavailable = []json.RawMessage{}
	}
	return json.Marshal(map[string]any{"postalPrefix": prefix, "tiers": tiers, "unavailable": q.Unavailable})
}

// decodeStrict decodes one JSON object and refuses unknown fields and
// trailing data.
func decodeStrict(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	return nil
}

func jsonReply(status int, body []byte) composition.Reply {
	return composition.Reply{Status: status, ContentType: "application/json", Body: body}
}
