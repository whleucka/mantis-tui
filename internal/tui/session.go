package tui

import (
	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
	"github.com/whleucka/mantis-tui/internal/service"
)

// Session is everything needed to talk to one host.
type Session struct {
	Host    config.Host
	API     mantis.API
	Meta    *meta.Cache
	Resolve *service.Resolver
}

// NewSession wires a session for host on top of api.
func NewSession(host config.Host, api mantis.API) *Session {
	m := meta.New(api)
	return &Session{Host: host, API: api, Meta: m, Resolve: service.NewResolver(m)}
}
