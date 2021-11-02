package dcache

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func init() {
	plugin.Register(name, setup)
}

func setup(c *caddy.Controller) error {
	var log = clog.NewWithPlugin(name)
	for c.Next() {
		if !c.NextArg() {
			return plugin.Error("dcache", c.ArgErr())
		}
	}

	dcache := New("localhost:6379")
	dcache.run()

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		dcache.Next = next
		dcache.log = log
		return dcache
	})

	return nil
}
