package signalsapi

// CommerceBeaconPaths are the /t routes core serves itself - a lead, a consent record and a restock subscription all need core's subscriber graph
// The beacon tier forwards exactly these and answers everything else, so neither side can grow a path the other misses
var CommerceBeaconPaths = []string{"/t/back-in-stock", "/t/forms", "/t/forms/submit"}
