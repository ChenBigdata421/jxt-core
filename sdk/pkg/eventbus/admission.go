package eventbus

// AdmissionOutcome is the result of admitting a broker ACK result into the registry.
// The registry is the ONLY layer allowed to non-blockingly reject; everywhere downstream
// of an Accepted result must drain losslessly.
type AdmissionOutcome int

const (
	AdmissionRejectedUnspecified  AdmissionOutcome = iota // must not be emitted
	AdmissionAccepted                                     // reached a tenant or global listener channel
	AdmissionRejectedFull                                 // tenant channel full (listener too slow)
	AdmissionRejectedFrozen                               // registry is shutting down (closed)
	AdmissionRejectedUnregistered                         // tenant not registered AND no global listener
)
