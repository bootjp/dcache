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

	dcache := New("127.0.0.1:6379")
	dcache.log = log

	if err := dcache.connect(); err != nil {
		return err
	}

	log.Infof("redis connect success")

	go dcache.run()

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		dcache.Next = next
		return dcache
	})

	return nil
}
