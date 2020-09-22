package nameserver

import (
	"context"
	"fmt"

	"github.com/miekg/dns"
	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/nameserver/nsutil"
)

// sendResponse sends a response to query using w. The response message is
// created by responder. If addExtraRRs is not nil and implements the
// RRProvider interface then it will be also used to add more RRs in the
// extra section.
func sendResponse(
	ctx context.Context,
	w dns.ResponseWriter,
	request *dns.Msg,
	responder nsutil.Responder,
	rrProviders ...nsutil.RRProvider,
) error {
	// Have the Responder craft a DNS reply.
	reply := responder.ReplyWithDNS(ctx, request)
	if reply == nil {
		// Dropping query.
		return nil
	}

	// Add extra RRs through a custom RRProvider.
	for _, rrProvider := range rrProviders {
		rrs := rrProvider.GetExtraRRs(ctx, request)
		reply.Extra = append(reply.Extra, rrs...)
	}

	// Write reply.
	if err := writeDNSResponse(w, reply); err != nil {
		return fmt.Errorf("nameserver: failed to send response: %w", err)
	}

	return nil
}

func writeDNSResponse(w dns.ResponseWriter, m *dns.Msg) (err error) {
	defer func() {
		// recover from panic
		if panicErr := recover(); panicErr != nil {
			err = fmt.Errorf("panic: %s", panicErr)
			log.Warningf("nameserver: panic caused by this msg: %#v", m)
		}
	}()

	err = w.WriteMsg(m)
	return
}
