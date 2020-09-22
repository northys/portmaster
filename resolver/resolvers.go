package resolver

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/publicsuffix"

	"github.com/safing/portbase/log"
	"github.com/safing/portmaster/netenv"
	"github.com/safing/portmaster/network/netutils"
)

// Scope defines a domain scope and which resolvers can resolve it.
type Scope struct {
	Domain    string
	Resolvers []*Resolver
}

var (
	globalResolvers []*Resolver          // all (global) resolvers
	localResolvers  []*Resolver          // all resolvers that are in site-local or link-local IP ranges
	systemResolvers []*Resolver          // all resolvers that were assigned by the system
	localScopes     []*Scope             // list of scopes with a list of local resolvers that can resolve the scope
	activeResolvers map[string]*Resolver // lookup map of all resolvers
	resolversLock   sync.RWMutex
)

func indexOfScope(domain string, list []*Scope) int {
	for k, v := range list {
		if v.Domain == domain {
			return k
		}
	}
	return -1
}

func getActiveResolverByIDWithLocking(server string) *Resolver {
	resolversLock.RLock()
	defer resolversLock.RUnlock()

	resolver, ok := activeResolvers[server]
	if ok {
		return resolver
	}
	return nil
}

func formatIPAndPort(ip net.IP, port uint16) string {
	var address string
	if ipv4 := ip.To4(); ipv4 != nil {
		address = fmt.Sprintf("%s:%d", ipv4.String(), port)
	} else {
		address = fmt.Sprintf("[%s]:%d", ip.String(), port)
	}
	return address
}

func resolverConnFactory(resolver *Resolver) ResolverConn {
	switch resolver.ServerType {
	case ServerTypeTCP:
		return NewTCPResolver(resolver)
	case ServerTypeDoT:
		return NewTCPResolver(resolver).UseTLS()
	case ServerTypeDNS:
		return NewPlainResolver(resolver)
	default:
		return nil
	}
}

func createResolver(resolverURL, source string) (*Resolver, bool, error) {
	u, err := url.Parse(resolverURL)
	if err != nil {
		return nil, false, err
	}

	switch u.Scheme {
	case ServerTypeDNS, ServerTypeDoT, ServerTypeTCP:
	default:
		return nil, false, fmt.Errorf("invalid DNS resolver scheme %q", u.Scheme)
	}

	ip := net.ParseIP(u.Hostname())
	if ip == nil {
		return nil, false, fmt.Errorf("invalid resolver IP")
	}

	// Add default port for scheme if it is missing.
	if u.Port() == "" {
		switch u.Scheme {
		case ServerTypeDNS, ServerTypeTCP:
			u.Host += ":53"
		case ServerTypeDoT:
			u.Host += ":853"
		}
	}

	scope := netutils.ClassifyIP(ip)
	if scope == netutils.HostLocal {
		return nil, true, nil // skip
	}

	query := u.Query()
	verifyDomain := query.Get("verify")
	if verifyDomain != "" && u.Scheme != ServerTypeDoT {
		return nil, false, fmt.Errorf("domain verification only supported in DOT")
	}

	if verifyDomain == "" && u.Scheme == ServerTypeDoT {
		return nil, false, fmt.Errorf("DOT must have a verify query parameter set")
	}

	blockType := query.Get("blockedif")
	if blockType == "" {
		blockType = BlockDetectionRefused
	}

	switch blockType {
	case BlockDetectionDisabled, BlockDetectionEmptyAnswer, BlockDetectionRefused, BlockDetectionZeroIP:
	default:
		return nil, false, fmt.Errorf("invalid value for upstream block detection (blockedif=)")
	}

	new := &Resolver{
		Server:                 resolverURL,
		ServerType:             u.Scheme,
		ServerAddress:          u.Host,
		ServerIP:               ip,
		ServerIPScope:          scope,
		Source:                 source,
		VerifyDomain:           verifyDomain,
		Name:                   query.Get("name"),
		UpstreamBlockDetection: blockType,
	}

	u.RawQuery = "" // Remove options from parsed URL
	if new.Name != "" {
		new.ServerInfo = fmt.Sprintf("%s (%s, from %s)", new.Name, u, source)
	} else {
		new.ServerInfo = fmt.Sprintf("%s (from %s)", u, source)
	}

	new.Conn = resolverConnFactory(new)
	return new, false, nil
}

func configureSearchDomains(resolver *Resolver, searches []string) {
	// only allow searches for local resolvers
	for _, value := range searches {
		trimmedDomain := strings.Trim(value, ".")
		if checkSearchScope(trimmedDomain) {
			resolver.Search = append(resolver.Search, fmt.Sprintf(".%s.", strings.Trim(value, ".")))
		}
	}
	// cap to mitigate exploitation via malicious local resolver
	if len(resolver.Search) > 100 {
		resolver.Search = resolver.Search[:100]
	}
}

func getConfiguredResolvers(list []string) (resolvers []*Resolver) {
	for _, server := range list {
		resolver, skip, err := createResolver(server, ServerSourceConfigured)
		if err != nil {
			// TODO(ppacher): module error
			log.Errorf("cannot use resolver %s: %s", server, err)
			continue
		}

		if skip {
			continue
		}

		resolvers = append(resolvers, resolver)
	}
	return resolvers
}

func getSystemResolvers() (resolvers []*Resolver) {
	for _, nameserver := range netenv.Nameservers() {
		serverURL := fmt.Sprintf("dns://%s", formatIPAndPort(nameserver.IP, 53))
		resolver, skip, err := createResolver(serverURL, ServerSourceOperatingSystem)
		if err != nil {
			// that shouldn't happen but handle it anyway ...
			log.Errorf("cannot use system resolver %s: %s", serverURL, err)
			continue
		}

		if skip {
			continue
		}

		if netutils.IPIsLAN(nameserver.IP) {
			configureSearchDomains(resolver, nameserver.Search)
		}

		resolvers = append(resolvers, resolver)
	}
	return resolvers
}

func loadResolvers() {
	// TODO: what happens when a lot of processes want to reload at once? we do not need to run this multiple times in a short time frame.
	resolversLock.Lock()
	defer resolversLock.Unlock()

	newResolvers := append(
		getConfiguredResolvers(configuredNameServers()),
		getSystemResolvers()...,
	)

	if len(newResolvers) == 0 {
		msg := "no (valid) dns servers found in (user) configuration or system, falling back to defaults"
		log.Warningf("resolver: %s", msg)
		module.Warning("no-valid-user-resolvers", msg)

		// load defaults directly, overriding config system
		newResolvers = getConfiguredResolvers(defaultNameServers)
		if len(newResolvers) == 0 {
			msg = "no (valid) dns servers found in configuration or system"
			log.Criticalf("resolver: %s", msg)
			module.Error("no-valid-default-resolvers", msg)
			return
		}
	}

	// save resolvers
	globalResolvers = newResolvers

	// assing resolvers to scopes
	setScopedResolvers(globalResolvers)

	// set active resolvers (for cache validation)
	// reset
	activeResolvers = make(map[string]*Resolver)
	// add
	for _, resolver := range newResolvers {
		activeResolvers[resolver.Server] = resolver
	}
	activeResolvers[mDNSResolver.Server] = mDNSResolver
	activeResolvers[envResolver.Server] = envResolver

	// log global resolvers
	if len(globalResolvers) > 0 {
		log.Trace("resolver: loaded global resolvers:")
		for _, resolver := range globalResolvers {
			log.Tracef("resolver: %s", resolver.Server)
		}
	} else {
		log.Warning("resolver: no global resolvers loaded")
	}

	// log local resolvers
	if len(localResolvers) > 0 {
		log.Trace("resolver: loaded local resolvers:")
		for _, resolver := range localResolvers {
			log.Tracef("resolver: %s", resolver.Server)
		}
	} else {
		log.Info("resolver: no local resolvers loaded")
	}

	// log system resolvers
	if len(systemResolvers) > 0 {
		log.Trace("resolver: loaded system/network-assigned resolvers:")
		for _, resolver := range systemResolvers {
			log.Tracef("resolver: %s", resolver.Server)
		}
	} else {
		log.Info("resolver: no system/network-assigned resolvers loaded")
	}

	// log scopes
	if len(localScopes) > 0 {
		log.Trace("resolver: loaded scopes:")
		for _, scope := range localScopes {
			var scopeServers []string
			for _, resolver := range scope.Resolvers {
				scopeServers = append(scopeServers, resolver.Server)
			}
			log.Tracef("resolver: %s: %s", scope.Domain, strings.Join(scopeServers, ", "))
		}
	} else {
		log.Info("resolver: no scopes loaded")
	}

	// alert if no resolvers are loaded
	if len(globalResolvers) == 0 && len(localResolvers) == 0 {
		log.Critical("resolver: no resolvers loaded!")
	}
}

func setScopedResolvers(resolvers []*Resolver) {
	// make list with local resolvers
	localResolvers = make([]*Resolver, 0)
	systemResolvers = make([]*Resolver, 0)
	localScopes = make([]*Scope, 0)

	for _, resolver := range resolvers {
		if resolver.ServerIP != nil && netutils.IPIsLAN(resolver.ServerIP) {
			localResolvers = append(localResolvers, resolver)
		}

		if resolver.Source == ServerSourceOperatingSystem {
			systemResolvers = append(systemResolvers, resolver)
		}

		if resolver.Search != nil {
			// add resolver to custom searches
			for _, search := range resolver.Search {
				if search == "." {
					continue
				}
				key := indexOfScope(search, localScopes)
				if key == -1 {
					localScopes = append(localScopes, &Scope{
						Domain:    search,
						Resolvers: []*Resolver{resolver},
					})
					continue
				}
				localScopes[key].Resolvers = append(localScopes[key].Resolvers, resolver)
			}
		}
	}

	// sort scopes by length
	sort.Slice(localScopes,
		func(i, j int) bool {
			return len(localScopes[i].Domain) > len(localScopes[j].Domain)
		},
	)
}

func checkSearchScope(searchDomain string) (ok bool) {
	// sanity check
	if len(searchDomain) == 0 ||
		searchDomain[0] == '.' ||
		searchDomain[len(searchDomain)-1] == '.' {
		return false
	}

	// add more subdomains to use official publicsuffix package for our cause
	searchDomain = "*.*.*.*.*." + searchDomain

	// get suffix
	suffix, icann := publicsuffix.PublicSuffix(searchDomain)
	// sanity check
	if len(suffix) == 0 {
		return false
	}
	// inexistent (custom) tlds are okay
	// this will include special service domains! (.onion, .bit, ...)
	if !icann && !strings.Contains(suffix, ".") {
		return true
	}

	// check if suffix is a special service domain (may be handled fully by local nameserver)
	if domainInScope("."+suffix+".", specialServiceDomains) {
		return true
	}

	// build eTLD+1
	split := len(searchDomain) - len(suffix) - 1
	eTLDplus1 := searchDomain[1+strings.LastIndex(searchDomain[:split], "."):]

	// scope check
	//nolint:gosimple // want comment
	if strings.Contains(eTLDplus1, "*") {
		// oops, search domain is too high up the hierarchy
		return false
	}

	return true
}
