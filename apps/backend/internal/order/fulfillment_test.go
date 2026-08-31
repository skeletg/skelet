package order

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
)

func TestPlanFulfillment(t *testing.T) {
	lineA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	lineB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	stranger := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	tests := []struct {
		name        string
		ordered     map[uuid.UUID]float64
		shipped     map[uuid.UUID]float64
		req         []FulfillmentLineInput
		wantErrCode string // "" => success
		wantFully   bool
		wantReq     map[uuid.UUID]float64
	}{
		{
			name:      "partial shipment of one line",
			ordered:   map[uuid.UUID]float64{lineA: 5, lineB: 3},
			shipped:   nil,
			req:       []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 2}},
			wantFully: false,
			wantReq:   map[uuid.UUID]float64{lineA: 2},
		},
		{
			name:      "ship remainder completes order",
			ordered:   map[uuid.UUID]float64{lineA: 5, lineB: 3},
			shipped:   map[uuid.UUID]float64{lineA: 2},
			req:       []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 3}, {OrderLineID: lineB, Quantity: 3}},
			wantFully: true,
			wantReq:   map[uuid.UUID]float64{lineA: 3, lineB: 3},
		},
		{
			name:      "ship everything in one shot",
			ordered:   map[uuid.UUID]float64{lineA: 5, lineB: 3},
			shipped:   nil,
			req:       []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 5}, {OrderLineID: lineB, Quantity: 3}},
			wantFully: true,
			wantReq:   map[uuid.UUID]float64{lineA: 5, lineB: 3},
		},
		{
			name:      "duplicate lines accumulate",
			ordered:   map[uuid.UUID]float64{lineA: 5},
			shipped:   nil,
			req:       []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 2}, {OrderLineID: lineA, Quantity: 3}},
			wantFully: true,
			wantReq:   map[uuid.UUID]float64{lineA: 5},
		},
		{
			name:      "fractional quantity is honored",
			ordered:   map[uuid.UUID]float64{lineA: 2.5},
			shipped:   nil,
			req:       []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 2.5}},
			wantFully: true,
			wantReq:   map[uuid.UUID]float64{lineA: 2.5},
		},
		{
			name:        "over-ship in one request is rejected",
			ordered:     map[uuid.UUID]float64{lineA: 5},
			shipped:     nil,
			req:         []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 6}},
			wantErrCode: "FULFILLMENT_OVERSHIP",
		},
		{
			name:        "over-ship against prior shipments is rejected",
			ordered:     map[uuid.UUID]float64{lineA: 5},
			shipped:     map[uuid.UUID]float64{lineA: 4},
			req:         []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 2}},
			wantErrCode: "FULFILLMENT_OVERSHIP",
		},
		{
			name:        "duplicate lines that together over-ship are rejected",
			ordered:     map[uuid.UUID]float64{lineA: 5},
			shipped:     nil,
			req:         []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 3}, {OrderLineID: lineA, Quantity: 3}},
			wantErrCode: "FULFILLMENT_OVERSHIP",
		},
		{
			name:        "unknown line is rejected",
			ordered:     map[uuid.UUID]float64{lineA: 5},
			shipped:     nil,
			req:         []FulfillmentLineInput{{OrderLineID: stranger, Quantity: 1}},
			wantErrCode: "FULFILLMENT_UNKNOWN_LINE",
		},
		{
			name:        "zero quantity is rejected",
			ordered:     map[uuid.UUID]float64{lineA: 5},
			shipped:     nil,
			req:         []FulfillmentLineInput{{OrderLineID: lineA, Quantity: 0}},
			wantErrCode: "FULFILLMENT_INVALID_QTY",
		},
		{
			name:        "negative quantity is rejected",
			ordered:     map[uuid.UUID]float64{lineA: 5},
			shipped:     nil,
			req:         []FulfillmentLineInput{{OrderLineID: lineA, Quantity: -1}},
			wantErrCode: "FULFILLMENT_INVALID_QTY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, fully, err := planFulfillment(tt.ordered, tt.shipped, tt.req)
			if tt.wantErrCode != "" {
				if err == nil {
					t.Fatalf("planFulfillment() = nil error, want code %q", tt.wantErrCode)
				}
				if code := apperr.CodeOf(err); code != tt.wantErrCode {
					t.Fatalf("planFulfillment() error code = %q, want %q", code, tt.wantErrCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("planFulfillment() unexpected error: %v", err)
			}
			if fully != tt.wantFully {
				t.Errorf("fully = %v, want %v", fully, tt.wantFully)
			}
			if len(req) != len(tt.wantReq) {
				t.Fatalf("reqByLine = %v, want %v", req, tt.wantReq)
			}
			for k, want := range tt.wantReq {
				if req[k] != want {
					t.Errorf("reqByLine[%s] = %v, want %v", k, req[k], want)
				}
			}
		})
	}
}
