package network

// Verdict describes the decision made about a connection or link.
type Verdict int8

// List of values a Status can have
const (
	// UNDECIDED is the default status of new connections
	VerdictUndecided           Verdict = 0
	VerdictUndeterminable      Verdict = 1
	VerdictAccept              Verdict = 2
	VerdictBlock               Verdict = 3
	VerdictDrop                Verdict = 4
	VerdictRerouteToNameserver Verdict = 5
	VerdictRerouteToTunnel     Verdict = 6
	VerdictFailed              Verdict = 7
)

func (v Verdict) String() string {
	switch v {
	case VerdictUndecided:
		return "<Undecided>"
	case VerdictUndeterminable:
		return "<Undeterminable>"
	case VerdictAccept:
		return "Accept"
	case VerdictBlock:
		return "Block"
	case VerdictDrop:
		return "Drop"
	case VerdictRerouteToNameserver:
		return "RerouteToNameserver"
	case VerdictRerouteToTunnel:
		return "RerouteToTunnel"
	case VerdictFailed:
		return "Failed"
	default:
		return "<INVALID VERDICT>"
	}
}

// Verb returns the verdict as a past tense verb.
func (v Verdict) Verb() string {
	switch v {
	case VerdictUndecided:
		return "undecided"
	case VerdictUndeterminable:
		return "undeterminable"
	case VerdictAccept:
		return "accepted"
	case VerdictBlock:
		return "blocked"
	case VerdictDrop:
		return "dropped"
	case VerdictRerouteToNameserver:
		return "rerouted to nameserver"
	case VerdictRerouteToTunnel:
		return "rerouted to tunnel"
	case VerdictFailed:
		return "failed"
	default:
		return "invalid"
	}
}

// Packer Directions
const (
	Inbound  = true
	Outbound = false
)

// Non-Domain Scopes
const (
	IncomingHost     = "IH"
	IncomingLAN      = "IL"
	IncomingInternet = "II"
	IncomingInvalid  = "IX"
	PeerHost         = "PH"
	PeerLAN          = "PL"
	PeerInternet     = "PI"
	PeerInvalid      = "PX"
)
