// Package syncveto is the shared reject/reason half of a sync webhook's response, so every "-ing" event reads a veto the same way
package syncveto

import "encoding/json"

// Embed it in an event's response struct - encoding/json promotes the fields, so the wire names stay "reject" and "reason"
type Response struct {
	Reject bool   `json:"reject"`
	Reason string `json:"reason"`
}

// Interprets a veto-only response - an empty body is "no opinion" and allows, while {"reject": true, "reason": "..."} vetoes
func Decode(body []byte) (rejected bool, reason string, err error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	return false, "", nil
}
